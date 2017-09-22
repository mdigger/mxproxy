package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/mdigger/log"
	"golang.org/x/crypto/pkcs12"
	"golang.org/x/net/http2"
)

// PushTimeout задает максимальное время ожидания ответа при посылке уведомлений.
var PushTimeout = time.Second * 30

// Push описывает конфигурация для отправки уведомлений через сервисы
// Apple Push Notification и Firebase Cloud Messaging.
type Push struct {
	apns  map[string]*http.Client // сертификаты для Apple Push
	fcm   map[string]string       // ключи для Firebase Cloud Messages
	store *Store                  // хранилище токенов
}

// Send отсылает уведомление на все устройства пользователя.
func (p *Push) Send(login string, obj interface{}) {
	// запускаем параллельно отсылку пушей
	go log.IfErr(p.sendAPN(login, obj), "send Apple Notification error")
	go log.IfErr(p.sendFCM(login, obj), "send Firebase Cloud Messages error")
}

// sendAPN отсылает уведомление на все Apple устройства пользователя.
func (p *Push) sendAPN(login string, obj interface{}) error {
	// преобразуем данные для пуша в формат JSON
	var payload []byte
	switch obj := obj.(type) {
	case []byte:
		payload = obj
	case string:
		payload = []byte(obj)
	case json.RawMessage:
		payload = []byte(obj)
	default:
		var err error
		payload, err = json.Marshal(obj)
		if log.IfErr(err, "push payload to json error") != nil {
			return err
		}
	}
	for topic, client := range p.apns {
		// получаем список токенов пользователя для данного сертификата
		var tokens = p.store.ListTokens("apn", topic, login)
		if len(tokens) == 0 {
			continue
		}
		// задаем хост в зависимости от sandbox
		var host string
		if topic[len(topic)-1] != '~' {
			host = "https://api.push.apple.com"
		} else {
			host = "https://api.development.push.apple.com"
		}
		// для каждого токена устройства формируем отдельный запрос
		var success, failure int // счетчики
		for _, token := range tokens {
			req, err := http.NewRequest("POST", host+"/3/device/"+token,
				bytes.NewReader(payload))
			if err != nil {
				return err
			}
			req.Header.Set("user-agent", agent)
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if log.IfErr(err, "apple push send error") != nil {
				failure++
				tlgrm.IfErr(err, "apple push send error",
					"login", login,
					"topic", topic,
					"token", token)
				continue
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
				continue
			}
			// в случае ошибки связанной с токеном устройства, удаляем его
			switch apnsError.Reason {
			case "MissingDeviceToken",
				"BadDeviceToken",
				"DeviceTokenNotForTopic",
				"Unregistered":
				p.store.RemoveToken("apn", topic, token)
			default:
			}
			log.Debug("apple push error",
				"topic", topic,
				"token", token,
				"reason", apnsError.Reason)
		}
		log.Info("apple push",
			"topic", topic,
			"success", success,
			"failure", failure)
	}
	return nil
}

var fcmClient = &http.Client{Timeout: PushTimeout}

// sendFCM отсылает уведомление на все Google устройства пользователя.
func (p *Push) sendFCM(login string, obj interface{}) error {
	for appName, fcmKey := range p.fcm {
		// получаем список токенов пользователя для данного сертификата
		var tokens = p.store.ListTokens("fcm", appName, login)
		if len(tokens) == 0 {
			continue
		}
		// формируем данные для отправки (без визуальной составляющей пуша:
		// только данные)
		var gfcmMsg = &struct {
			RegistrationIDs []string    `json:"registration_ids,omitempty"`
			Data            interface{} `json:"data,omitempty"`
			TTL             uint16      `json:"time_to_live"`
		}{
			// т.к. тут только устройства ОДНОГО пользователя, то
			// ограничением на количество токенов можно пренебречь
			RegistrationIDs: tokens,
			Data:            obj, // добавляем уже сформированные ранее данные
			// время жизни сообщения TTL = 0, поэтому оно не кешируется
			// на сервере, а сразу отправляется пользователю: для пушей
			// оо звонках мне показалось это наиболее актуальным.
		}
		// приводим к формату JSON
		data, err := json.Marshal(gfcmMsg)
		if err != nil {
			return err
		}
		req, err := http.NewRequest("POST",
			"https://fcm.googleapis.com/fcm/send", bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", agent)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "key="+fcmKey)
		resp, err := fcmClient.Do(req)
		if err != nil {
			tlgrm.IfErr(err, "google push send error",
				"login", login,
				"appName", appName,
				"tokens", tokens)
			return err
		}
		// проверяем статус ответа
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return err
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
			return err
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
					p.store.RemoveToken("fcm", appName, token)
					p.store.AddToken("fcm", appName, result.RegistrationID, login)
				}
			case "Unavailable":
				// устройство в данный момент не доступно
			default:
				// все остальное представляет из себя, так или иначе,
				// ошибки, связанные с неверным токеном устройства
				token := gfcmMsg.RegistrationIDs[indx]
				p.store.RemoveToken("fcm", appName, token)
			}
		}
		log.Info("google push",
			"app", appName,
			"success", result.Success,
			"failure", result.Failure)
	}
	return nil
}

// Support возвращает true, если данная тема поддерживается в качестве
// уведомления.
func (p *Push) Support(kind, topic string) bool {
	switch kind {
	case "apn":
		_, ok := p.apns[topic]
		return ok
	case "fcm":
		_, ok := p.fcm[topic]
		return ok
	default:
		return false
	}
}

// LoadCertificate загружает сертификат для Apple Push и сохраняем во внутреннем
// списке подготовленный для него http.Transport.
func (p *Push) LoadCertificate(filename, password string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	privateKey, x509Cert, err := pkcs12.Decode(data, password)
	if err != nil {
		return err
	}
	if _, err = x509Cert.Verify(x509.VerifyOptions{}); err != nil {
		if _, ok := err.(x509.UnknownAuthorityError); !ok {
			return err
		}
	}
	var topicID string
	for _, attr := range x509Cert.Subject.Names {
		if attr.Type.Equal(typeBundle) {
			topicID = attr.Value.(string)
			break
		}
	}
	var transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{
				tls.Certificate{
					Certificate: [][]byte{x509Cert.Raw},
					PrivateKey:  privateKey,
					Leaf:        nil,
				},
			},
		},
		// MaxIdleConns:    10,
		// MaxIdleConnsPerHost: 2,
		IdleConnTimeout: time.Minute * 10,
	}
	if err = http2.ConfigureTransport(transport); err != nil {
		return err
	}
	var client = &http.Client{
		Timeout:   PushTimeout,
		Transport: transport,
	}
	if p.apns == nil {
		p.apns = make(map[string]*http.Client)
	}
	for _, attr := range x509Cert.Extensions {
		switch t := attr.Id; {
		case t.Equal(typeDevelopmet): // Development
			p.apns[topicID+"~"] = client
		case t.Equal(typeProduction): // Production
			p.apns[topicID] = client
		case t.Equal(typeTopics): // Topics
			// не поддерживаем сертификаты с несколькими темами, т.к. для них
			// нужна более сложная работа
			return errors.New("apns certificate with topics not supported")
		}
	}
	log.Info("apple push certificate",
		"file", filename,
		"topic", topicID,
		"expire", x509Cert.NotAfter.Format("2006-01-02"))
	return nil
}

var (
	typeBundle     = asn1.ObjectIdentifier{0, 9, 2342, 19200300, 100, 1, 1}
	typeDevelopmet = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 3, 1}
	typeProduction = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 3, 2}
	typeTopics     = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 3, 6}
)
