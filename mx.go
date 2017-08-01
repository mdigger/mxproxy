package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/mdigger/log"
	"github.com/mdigger/mxproxy/csta"
)

// MXReadTimeout используется в качестве максимального времени ожидания ответа
// от сервера MX при запросах.
var MXReadTimeout = time.Second

// MX описывает серверное соединение с MX.
type MX struct {
	MXConfig     // адрес сервера, логин и пароль
	*csta.Client // соединение с сервером

	contacts  map[csta.JID]*Contact // адресная книга
	mu        sync.RWMutex
	authCache MXAuthCache  // кеш авторизации пользователей
	callmons  CallMonitors // информация о мониторах входящих звонков
}

// Connect подключается к серверу MX и запрашивает адресную книгу.
func (mx *MX) Connect() error {
	// устанавливаем соединение
	client, err := csta.NewClient(mx.Addr, csta.Login{
		UserName: mx.Login,
		Password: mx.Password,
		Type:     "Server",
	})
	if err != nil {
		return err
	}
	// запрашиваем и получаем адресную книгу
	contacts, err := GetAddressBook(client)
	if err != nil {
		client.Close()
		return err
	}
	// сохраняем настройки
	mx.mu.Lock()
	mx.Client = client
	mx.contacts = contacts
	mx.mu.Unlock()
	// сбрасываем список мониторов
	mx.callmons.Clear()
	return nil
}

// Monitoring запускает процесс чтения и разбора ответов от сервера MX.
func (mx *MX) Monitoring() error {
	ctxlog := log.WithField("mx", mx.SN)
	// запускаем мониторинг изменений в адресной книге
	if _, err := mx.Send("<MonitorStartAb/>"); err != nil {
		return err
	}
	// запрашиваем в хранилище список пользователей для мониторинга
	// звонков на данном MX сервере
	if users, err := storeDB.Users(mx.SN); err == nil && len(users) > 0 {
		mx.CallMonsStart(users...) // запускаем мониторинг звонков
	}
	for {
		response, err := mx.Receive() // ожидаем ответа от сервера
		if err != nil {
			return err // соединение разорвано
		}
		switch response.Name {
		case "AbUpdateUserEvent", "AbAddUserEvent":
			// добавление/изменения пользователя в адресной книге
			var update = new(struct {
				Contact Contact `xml:"abentry"`
			})
			if err := response.Decode(update); err != nil {
				return err
			}
			mx.mu.Lock()
			mx.contacts[update.Contact.JID] = &update.Contact
			mx.mu.Unlock()
			ctxlog.WithField("jid", update.Contact.JID).Debug("contact changed")
		case "AbDeleteUserEvent":
			// удаление пользователя из адресной книги
			var update = new(struct {
				JID csta.JID `xml:"userId"`
			})
			if err := response.Decode(update); err != nil {
				return err
			}
			mx.mu.Lock()
			delete(mx.contacts, update.JID)
			mx.mu.Unlock()
			ctxlog.WithField("jid", update.JID).Debug("contact deleted")
		case "MonitorStartResponse": // запущен монитор
			// разбираем идентификатор монитора
			var monitor = new(struct {
				ID uint64 `xml:"monitorCrossRefID"`
			})
			if err = response.Decode(monitor); err != nil {
				return err
			}
			mx.callmons.Started(response.ID, monitor.ID)
		case "CSTAErrorCode": // ошибка CSTA
			// TODO: разобрать ошибку запуска монитора
			mx.callmons.Started(response.ID, 0)
		case "DeliveredEvent":
			// входящий звонок
			var delivery = new(MXDelivery)
			if err := response.Decode(delivery); err != nil {
				return err
			}
			// добавляем временную метку, которой изначально нет
			delivery.Time = time.Now().UTC()
			// сопоставляем с монитором звонков пользователей
			jid := mx.callmons.JID(delivery.MonitorCrossRefID)
			if jid == 0 {
				ctxlog.WithField("id", delivery.MonitorCrossRefID).
					Warning("unknown delivery call monitor")
				continue
			}
			ctxlog.WithFields(log.Fields{
				"jid":      jid,
				"callId":   delivery.CallID,
				"alerting": delivery.AlertingDevice,
			}).Info("incoming call")
			// формируем уникальный идентификатор пользователя в хранилище
			userID := fmt.Sprintf("%s:%s", mx.SN, jid)
			// отправляем пуши на все зарегистрированные устройства пользователя
			// т.к. это может быть не быстрый процесс, то делаем это асинхронно
			go Push(userID, delivery)
		}
	}
}

// UserClient возвращает новое инициализированное пользовательское соединение
// с сервером MX. Информацию об успешной авторизации пользователя кешируется.
func (mx *MX) UserClient(login, password string) (*csta.Client, error) {
	client, err := csta.NewClient(mx.Addr, csta.Login{
		UserName: login,
		Password: password,
		Type:     "User",
		Platform: "iPhone",
		Version:  "3.0",
	})
	if err != nil {
		return nil, err
	}
	// в случае успешной авторизации добавляем логин и пароль в кеш.
	mx.authCache.Add(login, password, client.JID)
	return client, nil
}
