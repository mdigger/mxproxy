package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mdigger/log"
)

var (
	PushTimeout = time.Second * 5
	userAgent   = fmt.Sprintf("%s/%s", appName, version) // имя агента для пушей
	gfcmClient  = &http.Client{Timeout: PushTimeout}     // клиент для Google Push
)

// Push отсылает пуш уведомление на зарегистрированные устройства пользователя.
//
// Данный код расчитан на отправку исключительно пушей с данными и не
// поддерживает передачу визуальных сообщений. Например, VoIP-приложений.
func Push(userID string, payload interface{}) error {
	// преобразуем данные для пуша в формат JSON
	var dataPayload []byte
	switch data := payload.(type) {
	case []byte:
		dataPayload = data
	case string:
		dataPayload = []byte(data)
	case json.RawMessage:
		dataPayload = []byte(data)
	default:
		var err error
		dataPayload, err = json.Marshal(payload)
		if err != nil {
			log.WithError(err).Error("push payload to json error")
			return err
		}
	}
	ctxlog := log.WithField("user", userID)

	// получаем список зарегистрированных устройств пользователя
	userDeviceTokens, err := storeDB.Get(userID)
	if err != nil {
		log.WithError(err).Error("get users tokens from store error")
		return err
	}
	// разбираем токены устройств по их типу и bundle id
	for bundleID, tokens := range userDeviceTokens {
		// на всякий случай проверяем длину ключа, чтобы не было ошибок
		if len(tokens) == 0 {
			continue
		}
		ctxlog := ctxlog.WithField("bundleID", bundleID)
		// находим разделитель типа
		devider := strings.IndexByte(bundleID, ':')
		if devider < 0 || devider == len(bundleID)-1 {
			ctxlog.Warning("bad bundleID type")
			continue
		}
		// в зависимости от типов пушей различается их обработка
		switch bundleID[:devider] {
		case "apns": // Apple Push
			ctxlog := ctxlog.WithField("type", "apns")
			// запрашиваем инициализированный транспорт
			// если его нет, то данный тип приложения не поддерживается
			client, ok := apnsClients[bundleID[devider+1:]]
			if !ok {
				ctxlog.Warning("push bundle ignored")
				continue
			}

			// задаем хост в зависимости от sandbox
			var host string
			if bundleID[len(bundleID)-1] != '~' {
				host = "https://api.push.apple.com"
			} else {
				host = "https://api.development.push.apple.com"
			}
			// для каждого токена устройства формируем отдельный запрос
			var success, failure int // счетчики
			for token := range tokens {
				ctxlog := ctxlog.WithField("token", token)
				req, _ := http.NewRequest("POST", host+"/3/device/"+token,
					bytes.NewReader(dataPayload))
				req.Header.Set("user-agent", userAgent)
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if err != nil {
					// в случае ошибки отправки пуша прекращаем обработку
					// данного bundle
					ctxlog.WithError(err).Error("apple push error")
					break
				}
				if resp.StatusCode == http.StatusOK {
					resp.Body.Close()
					success++
					continue
				}
				failure++
				// разбираем ответ сервера с описанием ошибки
				var apnsError = new(struct {
					Reason string `json:"reason"`
				})
				err = json.NewDecoder(resp.Body).Decode(apnsError)
				resp.Body.Close()
				if err != nil {
					ctxlog.WithError(err).Error("apple push response decode error")
					continue
				}
				ctxlog = ctxlog.WithField("error", apnsError.Reason)
				// в случае ошибки связанной с токеном устройства, удаляем его
				switch apnsError.Reason {
				case "MissingDeviceToken",
					"BadDeviceToken",
					"DeviceTokenNotForTopic",
					"Unregistered":
					storeDB.Remove(bundleID, token)
					ctxlog.Debug("remove apple bad token")
				default:
					ctxlog.Error("apple push error")
				}
			}
			ctxlog.WithFields(log.Fields{
				"success": success,
				"failure": failure,
			}).Info("apple push sended")

		case "gfcm": // Google Cloud Message
			ctxlog := ctxlog.WithField("type", "gfcm")
			// получаем ключ для авторизации пуша для приложения
			// игнорируем все незарегистрированные типы приложений
			key, ok := gfcmKeys[bundleID[devider+1:]]
			if !ok {
				ctxlog.Warning("push bundle ignored")
				continue
			}

			// формируем список токенов устройств
			var bundleTokens = make([]string, 0, len(tokens))
			for token := range tokens {
				bundleTokens = append(bundleTokens, token)
			}
			// формируем данные для отправки (без визуальной составляющей пуша:
			// только данные)
			var gfcmMsg = &struct {
				RegistrationIDs []string        `json:"registration_ids,omitempty"`
				Data            json.RawMessage `json:"data,omitempty"`
				TTL             uint16          `json:"time_to_live"`
			}{
				// т.к. тут только устройства ОДНОГО пользователя, то
				// ограничением на количество токенов можно пренебречь
				RegistrationIDs: bundleTokens,
				// BUG(d3): данный хак позволяет не формировать второй раз
				// представление данных в формате JSON, но накладывает ограничения
				// на использование ИСКЛЮЧИТЕЛЬНО не визуальных данных
				Data: dataPayload, // добавляем уже сформированные ранее данные
				// время жизни сообщения TTL = 0, поэтому оно не кешируется
				// на сервере, а сразу отправляется пользователю: для пушей
				// оо звонках мне показалось это наиболее актуальным.
			}
			// приводим к формату JSON
			data, err := json.Marshal(gfcmMsg)
			if err != nil {
				ctxlog.WithError(err).Error("google push data create error")
				continue
			}
			req, _ := http.NewRequest("POST",
				"https://fcm.googleapis.com/fcm/send", bytes.NewReader(data))
			req.Header.Set("User-Agent", userAgent)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "key="+key)
			resp, err := gfcmClient.Do(req)
			if err != nil {
				ctxlog.WithError(err).Error("google push request error")
				continue
			}
			// проверяем статус ответа
			if resp.StatusCode != http.StatusOK {
				ctxlog.WithField("status", resp.StatusCode).
					Error("google push error")
				resp.Body.Close()
				continue
			}
			// разбираем ответ сервера
			var result = new(struct {
				Success int `json:"success"`
				Failure int `json:"failure"`
				Results []struct {
					RegistrationID string `json:"registration_id"`
					Error          string `json:"error"`
				} `json:"results"`
			})
			err = json.NewDecoder(resp.Body).Decode(result)
			resp.Body.Close()
			if err != nil {
				ctxlog.WithError(err).Error("google push response decode error")
				continue
			}
			// проходим по массиву результатов в ответе для каждого токена
			for indx, result := range result.Results {
				switch result.Error {
				case "":
					// нет ошибки - доставлено
					// проверяем, что, возможно, токен устарел и его нужно
					// заменить на более новый, который указан в ответе
					if result.RegistrationID != "" {
						token := gfcmMsg.RegistrationIDs[indx]
						ctxlog.WithField("token", token).
							Debug("update google push token")
						storeDB.Remove(bundleID, token)
						storeDB.Remove(bundleID, result.RegistrationID)
					}
				case "Unavailable":
					// устройство в данный момент не доступно
				default:
					// все остальное представляет из себя, так или иначе,
					// ошибки, связанные с неверным токеном устройства
					token := gfcmMsg.RegistrationIDs[indx]
					ctxlog.WithFields(log.Fields{
						"token": token,
						"error": result.Error,
					}).Debug("remove google bad token")
					storeDB.Remove(bundleID, token)
				}
			}
			ctxlog.WithFields(log.Fields{
				"success": result.Success,
				"failure": result.Failure,
			}).Info("google push sended")
		}
	}

	return nil
}
