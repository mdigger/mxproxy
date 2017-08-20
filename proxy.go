package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/mdigger/log"
	"github.com/mdigger/mx"
	"github.com/mdigger/rest"
	"github.com/yosuke-furukawa/json5/encoding/json5"
)

// ReconnectDelay задает время задержки между переподключением к серверу MX.
var ReconnectDelay = time.Minute

// Proxy описывает HTTP-прокси сервер для MX.
type Proxy struct {
	servers *MXList           // список серверных соединений MX
	apns    *APNS             // транспорт для отправки уведомлений Apple Push
	fcm     map[string]string // ключи для отправку уведомлений Firebase Cloud Message
	store   *Store            // хранилище токенов устройств
	stopped bool              // флаг остановки сервиса
	mu      sync.RWMutex
}

// LoadConfig загружает конфигурационный файл и на основании него инициализирует
// Proxy.
func LoadConfig(configName, tokensDBName string) (*Proxy, error) {
	data, err := ioutil.ReadFile(configName)
	if err != nil {
		return nil, err
	}
	// стуктура данных в конфигурационном файле представлена в следующем виде
	var config = new(struct {
		// список MX серверов c логинами и паролями для серверной авторизации
		MXList map[string]struct {
			Login    string `json:"login"`    // серверный логин
			Password string `json:"password"` // пароль
		} `json:"mx"`
		// временные ограничения
		Timeouts struct {
			Connect   Duration `json:"connectTimeout"`
			Read      Duration `json:"readTimeout"`
			Reconnect Duration `json:"reconnectDelay"`
			KeepAlive Duration `json:"keepAliveDuration"`
		} `json:"timeouts"`
		// список файлов с сертификатами APNS и паролями для их чтения
		APN map[string]string `json:"apn"`
		// список идентификаторов приложений Android и ассоциированных с ними
		// ключами
		FCM map[string]string `json:"fcm"`
	})
	if err = json5.Unmarshal(data, config); err != nil {
		return nil, err
	}
	log.WithField("file", configName).Info("config")

	// устанавливаем таймауты
	if config.Timeouts.Connect.Duration > 0 {
		mx.ConnectionTimeout = config.Timeouts.Connect.Duration
	}
	if config.Timeouts.Read.Duration > 0 {
		mx.ReadTimeout = config.Timeouts.Read.Duration
	}
	if config.Timeouts.KeepAlive.Duration > 0 {
		mx.KeepAliveDuration = config.Timeouts.KeepAlive.Duration
	}
	if config.Timeouts.Reconnect.Duration > 0 {
		ReconnectDelay = config.Timeouts.Reconnect.Duration
	}

	// загружаем сертификаты для Apple Push Notification
	var apns = new(APNS)
	for filename, password := range config.APN {
		if err := apns.LoadCertificate(filename, password); err != nil {
			return nil, err
		}
	}
	// сохраняем ключи для Firebase Cloud Messages
	var fcm = config.FCM
	for name := range fcm {
		log.WithField("app", name).Info("fcm application key")
	}
	// открываем хранилище токенов устройств для уведомлений
	store, err := OpenStore(tokensDBName)
	if err != nil {
		return nil, err
	}
	var mxlist = new(MXList)
	// инициализируем описание прокси
	var proxy = &Proxy{
		apns:    apns,
		fcm:     fcm,
		store:   store,
		servers: mxlist,
	}
	// инициализируем серверные соединения с MX серверами
	for host, user := range config.MXList {
		// проверяем, что у host указан порт или добавляем его, если нет
		if _, _, err = net.SplitHostPort(host); err != nil {
			if err, ok := err.(*net.AddrError); ok &&
				err.Err == "missing port in address" {
				host = net.JoinHostPort(host, "7778")
			} else {
				return nil, err
			}
		}
		mxs, err := ConnectMXServer(host, user.Login, user.Password)
		if err != nil {
			return nil, err
		}
		mxlist.Add(mxs) // сохраняем в списке

		// восстановление соединения в случае разрыва
		go func(mxs *MXServer) {
			for {
				// запускаем мониторы пользователей
				if jids, err := store.Users(mxs.ID()); err != nil {
					log.WithError(err).Warning("get mx user error")
				} else if err = mxs.MonitorStart(jids...); err != nil {
					log.WithError(err).Warning("starting users monitors error")
				}
				go mxs.CallMonitor(proxy.SendPush) // запускаем мониторинг звонков
				<-mxs.conn.Done()                  // ждем окончания соединения
			reconnect:
				// проверяем флаг, что сервис не остановлен планово
				proxy.mu.RLock()
				var stopped = proxy.stopped
				proxy.mu.RUnlock()
				if stopped {
					return
				}
				mxs.log.Infof("reconnect after %s...", ReconnectDelay)
				time.Sleep(ReconnectDelay) // задержка между подключениями
				newmxs, err := ConnectMXServer(mxs.host, mxs.login, mxs.password)
				if err != nil {
					mxs.log.WithError(err).Warning("mx server connection error")
					goto reconnect
				}
				mxlist.Add(newmxs) // сохраняем в списке
				mxs = newmxs       // подменяем старое соединение
			}
		}(mxs)
	}

	return proxy, nil
}

// Close закрывает все серверные соединения MX и хранилище токенов устройств.
func (p *Proxy) Close() {
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()
	p.servers.CloseAll()
	p.store.Close()
}

// getMXServer проверяет авторизацию пользователя и возвращает серверное
// соединение с MX.
func (p *Proxy) getMXServer(c *rest.Context) (*MXServer, error) {
	var mxs = p.servers.Get(c.Param("mx"))
	if mxs == nil {
		return nil, rest.ErrNotFound
	}
	login, password, ok := c.BasicAuth()
	if !ok {
		c.SetHeader("WWW-Authenticate", fmt.Sprintf("Basic realm=%s", appName))
		return nil, rest.ErrUnauthorized
	}
	c.AddLogField("login", login) // добавим логин в лог
	// проверяем авторизацию пользователя
	var jid = mxs.authCache.Check(login, password)
	if jid == 0 {
		log.WithFields(log.Fields{
			"login": login,
			"mx":    mxs.ID(),
			"type":  "user",
		}).Debug("check authorization")
		client, err := mxs.ConnectMXClient(login, password)
		if err != nil {
			// добавляем к ошибке код для http
			var status = http.StatusServiceUnavailable
			if _, ok := err.(*mx.LoginError); ok {
				status = http.StatusForbidden
			} else if err1, ok := err.(net.Error); ok && err1.Timeout() {
				status = http.StatusGatewayTimeout
			}
			return nil, rest.NewError(status, err.Error())
		}
		jid = client.conn.JID
		client.Close()
	}
	c.SetData("jid", jid)
	return mxs, nil
}

// getMXClient проверяет авторизацию пользователя и возвращает клиентское
// соединение с MX.
func (p *Proxy) getMXClient(c *rest.Context) (*MXClient, error) {
	var mxs = p.servers.Get(c.Param("mx"))
	if mxs == nil {
		return nil, rest.ErrNotFound
	}
	login, password, ok := c.BasicAuth()
	if !ok {
		c.SetHeader("WWW-Authenticate", fmt.Sprintf("Basic realm=%s", appName))
		return nil, rest.ErrUnauthorized
	}
	c.AddLogField("login", login) // добавим логин в лог
	conn, err := mxs.ConnectMXClient(login, password)
	if err == nil {
		return conn, nil
	}
	// добавляем к ошибке код для http
	var status = http.StatusServiceUnavailable
	if _, ok := err.(*mx.LoginError); ok {
		status = http.StatusForbidden
	} else if err1, ok := err.(net.Error); ok && err1.Timeout() {
		status = http.StatusGatewayTimeout
	}
	return nil, rest.NewError(status, err.Error())
}

// MXList описывает список серверных соединений MX.
type MXList struct {
	servers sync.Map
}

// Add регистрирует серверное соединение MX и сохраняет его в списке.
func (l *MXList) Add(mxs *MXServer) {
	l.servers.Store(mxs.ID(), mxs)
}

// Get возвращает из списка MXServer с указанным идентификатором.
func (l *MXList) Get(id string) *MXServer {
	if mxs, ok := l.servers.Load(id); ok {
		return mxs.(*MXServer)
	}
	return nil
}

// CloseAll останавливает все серверные соединения MX.
func (l *MXList) CloseAll() {
	l.servers.Range(func(id, mxs interface{}) bool {
		mxs.(*MXServer).Close()
		l.servers.Delete(id)
		return true
	})
}
