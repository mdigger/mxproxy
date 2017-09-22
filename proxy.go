package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/mdigger/log"
	"github.com/mdigger/log/telegram"
	"github.com/mdigger/mx"
	"github.com/mdigger/rest"
)

// Proxy описывает сервис проксирования запросов к серверу MX.
type Proxy struct {
	provisioningURL string            // адрес сервера провижининга
	appsAuth        map[string]string // связка client-id:secret
	store           *Store            // хранилище данных
	jwtGen          *JWTGenerator     // генератор авторизационных токенов
	conns           sync.Map          // пользовательские соединения с MX
	push            *Push             // отправитель уведомлений
	stopped         bool              // флаг остановки сервиса
	mu              sync.RWMutex
}

// лог для телеграмма - до инициализации пустышка
var tlgrm = log.NewLogger(log.Null)

// InitProxy инициализирует и возвращает сервис проксирования запросов к MX.
func InitProxy() (proxy *Proxy, err error) {
	// Telegram описывает настройки для чата Telegram
	type Telegram struct {
		Token  string `toml:"token"`  // токен для Telegram
		ChatID int64  `toml:"chatID"` // идентификатор чата
	}
	var config = &struct {
		ProvisioningURL string            `toml:"provisioning"`
		AppsAuth        map[string]string `toml:"apps"`
		DBName          string            `toml:"dbName"`
		LogName         string            `toml:"logName"`
		VoIP            struct {
			APN map[string]string `toml:"apn"`
			FCM map[string]string `toml:"fcm"`
		} `toml:"voip"`
		JWT struct {
			TokenTTL   string `toml:"tokenTTL"`   // время жизни токена
			SingKeyTTL string `toml:"signKeyTTL"` // время жизни ключа
		} `toml:"jwt"`
		Telegram Telegram `toml:"telegram"`
	}{
		ProvisioningURL: "https://config.connector73.net/config",
		DBName:          lowerAppName + ".db",
		Telegram: Telegram{
			Token:  "422160011:AAFz-BJhIFQLrdXI2L8BtxgvivDKeY5s2Ig",
			ChatID: -1001068031302,
		},
	}
	// разбираем конфигурационный файл, если он существует
	log.Info("loading configuration", "filename", configName)
	data, err := ioutil.ReadFile(configName)
	if err != nil {
		return nil, err
	}
	if err = toml.Unmarshal(data, config); err != nil {
		return nil, err
	}

	// задаем имя для поиска и отдачи лога приложения
	if config.LogName != "" {
		logFile = config.LogName
	}

	// инициализируем поддержку отправки ошибок через Telegram
	if config.Telegram.Token != "" && config.Telegram.ChatID != 0 &&
		// !strings.HasPrefix(host, "localhost") &&
		!strings.HasPrefix(host, "127.0.0.1") {
		var tlgrmhdlr = telegram.New(config.Telegram.Token,
			config.Telegram.ChatID, nil)
		tlgrmhdlr.Header = fmt.Sprintf("%s/%s", appName, version)
		tlgrmhdlr.Footer = fmt.Sprintf("----------------\n"+
			"Builded: %s\n"+
			"Git: %s\n"+
			"Host: %s",
			date, git, host)
		tlgrm = log.NewLogger(tlgrmhdlr)
	}

	// проверяем, что определены идентификаторы приложений для авторизации
	// OAuth2
	if len(config.AppsAuth) == 0 {
		return nil, errors.New("oauth2 apps not configured")
	}
	// выводим в лог список идентификаторов приложений
	var list = make([]string, 0, len(config.AppsAuth))
	for appName := range config.AppsAuth {
		list = append(list, appName)
	}
	sort.Strings(list)
	log.Info("registered oauth2 apps", "apps", strings.Join(list, ", "))

	// инициализируем генератор токенов авторизации
	var tokenTTL = time.Hour
	if config.JWT.TokenTTL != "" {
		d, err := time.ParseDuration(config.JWT.TokenTTL)
		if err != nil {
			return nil, err
		}
		tokenTTL = d
	}
	var singKeyTTL = time.Hour * 6
	if config.JWT.TokenTTL != "" {
		d, err := time.ParseDuration(config.JWT.TokenTTL)
		if err != nil {
			return nil, err
		}
		singKeyTTL = d
	}
	var jwtGen = NewJWTGenerator(tokenTTL, singKeyTTL)
	log.Info("token generator", "tokenTTL", tokenTTL, "signKeyTTL", singKeyTTL)

	// открываем хранилище
	store, err := OpenStore(config.DBName)
	if log.IfErr(err, "store error") != nil {
		return nil, err
	}

	// загружаем сертификаты для VoIP Apple Push
	var push = &Push{
		store: store,
		apns:  make(map[string]*http.Client, len(config.VoIP.APN)),
		fcm:   config.VoIP.FCM,
	}
	for filename, password := range config.VoIP.APN {
		log.IfErr(push.LoadCertificate(filename, password),
			"apn certificate error", "filename", filename)
	}
	// выводим список поддерживаемых приложений для Firebase Cloud Messages
	for appName := range config.VoIP.FCM {
		log.Info("firebase cloud messaging", "app", appName)
	}
	// инициализируем прокси
	proxy = &Proxy{
		provisioningURL: config.ProvisioningURL,
		appsAuth:        config.AppsAuth,
		store:           store,
		jwtGen:          jwtGen,
		push:            push,
	}
	// получаем список зарегистрированных пользователей и запускаем соединение
	for _, login := range store.ListUsers() {
		mxconf, _ := store.GetUser(login) // получаем конфигурацию
		// устанавливаем соединение
		if err = proxy.connect(mxconf, login); err != nil {
			// в случае ошибки авторизации удаляем пользователя
			if _, ok := err.(*mx.LoginError); ok {
				store.RemoveUser(login)
			}
			log.IfErr(err, "mx user connection error", "login", login)
		}
	}
	return proxy, nil
}

// Close останавливает все пользовательские соединения и закрывает хранилище.
func (p *Proxy) Close() error {
	p.mu.Lock()
	p.stopped = true // флаг остановки сервиса
	p.mu.Unlock()
	p.jwtGen.Close() // останавливаем удаление старых ключей
	p.conns.Range(func(login, conn interface{}) bool {
		p.conns.Delete(login)  // удаляем из списка
		conn.(*MXConn).Close() // останавливаем соединение
		return true
	})
	log.Info("proxy stopped")
	return p.store.Close()
}

// isStopped возвращает true, если сервис остановлен.
func (p *Proxy) isStopped() bool {
	p.mu.RLock()
	var result = p.stopped
	p.mu.RUnlock()
	return result
}

// connect осуществляет подключение пользователя к серверу MX.
func (p *Proxy) connect(conf *MXConfig, login string) error {
	conn, err := MXConnect(conf, login)
	if err != nil {
		log.IfErr(err, "mx user connection error")
		// в зависимости от типа ошибки возвращаем разный статус
		var status = http.StatusServiceUnavailable
		if _, ok := err.(*mx.LoginError); ok {
			status = http.StatusForbidden
		} else if errNet, ok := err.(net.Error); ok && errNet.Timeout() {
			status = http.StatusGatewayTimeout
		}
		return rest.NewError(status, err.Error())
	}
	p.conns.Store(login, conn) // сохраняем соединение в списке
	log.Info("mx user connected", "login", login)

	go func(conn *MXConn, login string) {
		ctxlog := log.New(login)
		ctxlog.Debug("mx user call monitoring")
		defer ctxlog.Debug("mx user call monitoring end")
	monitoring:
		// запускаем мониторинг входящих звонков
		err := conn.Handle(func(resp *mx.Response) error {
			var delivered = new(DeliveredEvent)
			if err := resp.Decode(delivered); err != nil {
				return err
			}
			// фильтруем события о звонках
			if delivered.CalledDevice != conn.Ext &&
				delivered.AlertingDevice != conn.Ext {
				ctxlog.Debug("ignore incoming call", "id", delivered.CallID)
				return nil
			}
			delivered.Timestamp = time.Now().Unix()
			p.push.Send(conn.Login, delivered) // отсылаем уведомление
			ctxlog.Info("incoming call", "id", delivered.CallID)
			return nil
		}, "DeliveredEvent")
		// проверяем, что сервис или соединение не остановлены
		if _, ok := p.conns.Load(login); p.isStopped() || !ok {
			return // сервис или соединение остановлены
		}
		ctxlog.IfErr(err, "monitoring error")
		// ждем окончания
		ctxlog.IfErr(<-conn.Done(), "mx user connection error")
		p.conns.Delete(login) // удаляем из списка соединений
	reconnect:
		conf, err = p.store.GetUser(login) // получаем конфигурацию
		if ctxlog.IfErr(err, "mx user config error") != nil {
			return
		}
		ctxlog.Debug("mx user reconnecting", "delay", time.Minute)
		time.Sleep(time.Minute) // задержка перед переподключением
		if p.isStopped() {
			return // сервис остановлен
		}
		conn, err = MXConnect(conf, login)
		if log.IfErr(err, "mx user connection error") != nil {
			// в случае ошибки авторизации удаляем пользователя
			if _, ok := err.(*mx.LoginError); ok {
				p.store.RemoveUser(login)
				return
			}
			goto reconnect
		}
		p.conns.Store(login, conn) // сохраняем соединение в списке
		ctxlog.Info("mx user connected")
		goto monitoring
	}(conn, login)

	return nil
}

// DeliveredEvent описывает структуру события входящего звонка
type DeliveredEvent struct {
	MonitorCrossRefID     int64  `xml:"monitorCrossRefID" json:"-"`
	CallID                int64  `xml:"connection>callID" json:"callId"`
	DeviceID              string `xml:"connection>deviceID" json:"deviceId"`
	GlobalCallID          string `xml:"connection>globalCallID" json:"globalCallId"`
	CallingDevice         string `xml:"callingDevice>deviceIdentifier" json:"callingDevice"`
	CalledDevice          string `xml:"calledDevice>deviceIdentifier" json:"calledDevice"`
	AlertingDevice        string `xml:"alertingDevice>deviceIdentifier" json:"alertingDevice"`
	LastRedirectionDevice string `xml:"lastRedirectionDevice>deviceIdentifier" json:"lastRedirectionDevice"`
	LocalConnectionInfo   string `xml:"localConnectionInfo" json:"localConnectionInfo"`
	Cause                 string `xml:"cause" json:"cause"`
	CallTypeFlags         int64  `xml:"callTypeFlags" json:"callTypeFlags,omitempty"`
	Timestamp             int64  `xml:"-" json:"timestamp"`
}

// Login проверяет авторизацию и возвращает авторизационный токен. Если
// пользовательское соединение с сервером MX не установлено, то устанавливает
// его. Данные для подключения к серверу MX сохраняются в хранилище.
func (p *Proxy) Login(c *rest.Context) error {
	// получаем информацию об авторизации из заголовка запроса
	clientID, secret, ok := c.BasicAuth()
	if !ok {
		c.SetHeader("WWW-Authenticate",
			fmt.Sprintf("Basic realm=%q", appName+" client application"))
		return rest.ErrUnauthorized
	}
	c.AddLogField("app", clientID)
	// авторизуем приложение
	if appSecret, ok := p.appsAuth[clientID]; !ok || appSecret != secret {
		return c.Error(http.StatusForbidden, "bad client-id or app secret")
	}
	// проверяем, что тип запроса соответствует OAuth2 спецификации
	if c.Form("grant_type") != "password" {
		return c.Error(http.StatusForbidden, "bad grant_type")
	}
	// получаем логин и пароль пользователя из запроса
	var login, password = c.Form("username"), c.Form("password")
	c.AddLogField("login", login) // добавим логин в лог
	// проверяем авторизацию на сервере провижининга и получаем конфигурацию
	mxconf, err := p.GetProvisioning(login, password)
	if err != nil {
		return err
	}
	// подключаемся к MX и авторизуем пользователя
	// TODO: проверить, что данные не изменились.
	if _, ok := p.conns.Load(login); !ok {
		if err = p.connect(mxconf, login); err != nil {
			return err
		}
	}
	// сохраняем информацию о пользователе в хранилище
	if err = p.store.AddUser(login, mxconf); err != nil {
		return err
	}
	// создаем токен на основании предоставленной информации и отдаем его
	tokenInfo, err := p.jwtGen.Token(login)
	if err != nil {
		return err
	}
	return c.Write(tokenInfo)
}

// Logout останавливает пользовательское соединение с сервером MX и удаляет
// информацию о пользователе из хранилища.
func (p *Proxy) Logout(c *rest.Context) error {
	// запрашивает токен авторизации из заголовка
	var auth = c.Header("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return rest.NewError(http.StatusForbidden,
			fmt.Sprintf("bearer authorization token required"))
	}
	// проверяем валидность токена и получаем логин пользователя
	login, err := p.jwtGen.Verify(strings.TrimPrefix(auth, "Bearer "))
	if err != nil {
		return rest.NewError(http.StatusForbidden,
			fmt.Sprintf("invalid token: %s", err.Error()))
	}
	c.AddLogField("login", login) // добавляем в лог
	// останавливаем соединение
	if conn, ok := p.conns.Load(login); ok {
		p.conns.Delete(login)  // удаляем из списка
		conn.(*MXConn).Close() // останавливаем соединение
	}
	// удаляем из хранилища
	if err = p.store.RemoveUser(login); err != nil {
		return err
	}
	log.WithField("login", login).Info("mx user disconnected")
	return c.Write(rest.JSON{"userLogout": login})
}

// getConnection проверяет токен с авторизацией пользователя и возвращает
// соединение с сервером MX.
func (p *Proxy) getConnection(c *rest.Context) (conn *MXConn, err error) {
	// в случае ошибки выставляем заголовок с требованием авторизации
	defer func() {
		if err != nil {
			c.SetHeader("WWW-Authenticate",
				fmt.Sprintf("Bearer realm=%q", appName))
		}
	}()
	// запрашивает токен авторизации из заголовка
	var auth = c.Header("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, rest.ErrUnauthorized
	}
	// проверяем валидность токена и получаем логин пользователя
	login, err := p.jwtGen.Verify(strings.TrimPrefix(auth, "Bearer "))
	if err != nil {
		return nil, rest.NewError(http.StatusUnauthorized,
			fmt.Sprintf("invalid token: %s", err.Error()))
	}
	c.AddLogField("login", login) // добавляем в лог
	// возвращаем соединение с сервером MX
	if conn, ok := p.conns.Load(login); ok {
		return conn.(*MXConn), nil
	}
	// возвращаем ошибку, что для данного пользователя нет активных
	// соединений с сервером MX
	return nil, rest.NewError(http.StatusUnauthorized,
		"active mx connection unavailable")
}

// LoginInfo возвращает информацию об авторизованном пользователе MX.
func (p *Proxy) LoginInfo(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	return c.Write(&struct {
		MX  string `json:"mx"`
		Ext string `json:"ext"`
		JID mx.JID `json:"jid,string"`
	}{
		MX:  conn.SN,
		Ext: conn.Ext,
		JID: conn.JID,
	})
}

// Contacts отдает адресную книгу сервера MX.
func (p *Proxy) Contacts(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	// получаем список контактов
	contacts, err := conn.Contacts()
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"contacts": contacts})
}

// CallLog отдает лог звонков пользователя.
func (p *Proxy) CallLog(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	var timestamp = c.Query("timestamp")
	var ts time.Time
	if timestamp != "" {
		if t, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
			ts = time.Unix(t, 0)
		} else if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
			ts = t
		} else {
			return c.Error(http.StatusBadRequest, "bad timestamp format")
		}
	}
	calllog, err := conn.CallLog(ts) // получаем лог звонков
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"callLog": calllog})
}

// SetMode устанавливает режим звонка.
func (p *Proxy) SetMode(c *rest.Context) error {
	conn, err := p.getConnection(c)
	if err != nil {
		return err
	}
	// Params описывает параметры, передаваемые в запроса
	type Params struct {
		Remote    bool   `json:"remote" form:"remote"`
		Device    string `json:"device" form:"device"`
		RingDelay uint16 `json:"ringDelay" form:"ringDelay"`
		VMDelay   uint16 `json:"vmDelay" form:"vmDelay"`
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = &Params{
		RingDelay: 1,
		VMDelay:   30,
	}
	if err = c.Bind(params); err != nil {
		return err
	}
	c.AddLogField("remote", params.Remote)
	c.AddLogField("device", params.Device)
	if err = conn.SetMode(params.Remote, params.Device,
		params.RingDelay, params.VMDelay); err != nil {
		return err
	}
	return c.Write(rest.JSON{"callMode": params})
}

// MakeCall отсылает команду на сервер MX об установке соединения между двумя
// указанными телефонами.
func (p *Proxy) MakeCall(c *rest.Context) error {
	conn, err := p.getConnection(c)
	if err != nil {
		return err
	}
	// Params описывает параметры, передаваемые в запроса
	var params = new(struct {
		From   string `json:"from" form:"from"`
		To     string `json:"to" form:"to"`
		Device string `json:"device" form:"device"`
	})
	if err = c.Bind(params); err != nil {
		return err
	}
	resp, err := conn.MakeCall(params.From, params.To, params.Device)
	if err != nil {
		if _, ok := err.(*mx.CSTAError); ok {
			return c.Error(http.StatusBadRequest, err.Error())
		}
		return err
	}
	return c.Write(rest.JSON{"makeCall": resp})
}

// AssignDevice ассоциирует имя устройства с пользовательской сессией.
func (p *Proxy) AssignDevice(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	var deviceID = c.Param("name")
	if deviceID == "" {
		return rest.ErrNotFound
	}
	return conn.AssignDevice(deviceID)
}

// SIPAnswer инициализирует поддержку SIP звонка.
func (p *Proxy) SIPAnswer(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	// Params описывает параметры, передаваемые в запроса
	callID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return rest.ErrNotFound
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = &struct {
		CallID  int64  `json:"callId" form:"callId"`
		Device  string `json:"device" form:"device"`
		Timeout uint16 `json:"timeout" form:"timeout"`
	}{
		Timeout: 30,
	}
	if err = c.Bind(params); err != nil {
		return err
	}
	if err = conn.SIPAnswer(callID, params.Device,
		time.Duration(params.Timeout)*time.Second); err != nil {
		return err
	}
	params.CallID = callID
	return c.Write(rest.JSON{"sipAnswer": params})
}

// Transfer перенаправляет звонок на другой номер.
func (p *Proxy) Transfer(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	// Params описывает параметры, передаваемые в запроса
	callID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return rest.ErrNotFound
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = new(struct {
		CallID int64  `json:"callId" form:"callId"`
		Device string `json:"device" form:"device"`
		To     string `json:"to" form:"to"`
	})
	if err = c.Bind(params); err != nil {
		return err
	}
	if err = conn.Transfer(callID, params.Device, params.To); err != nil {
		return err
	}
	params.CallID = callID
	return c.Write(rest.JSON{"transfer": params})
}

// Voicemails отдает список голосовых сообщений пользователя.
func (p *Proxy) Voicemails(c *rest.Context) error {
	conn, err := p.getConnection(c)
	if err != nil {
		return err
	}
	vmlist, err := conn.VoiceMailList()
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"voiceMails": vmlist})
}

// DeleteVoicemail удаляет голосовое сообщение.
func (p *Proxy) DeleteVoicemail(c *rest.Context) error {
	conn, err := p.getConnection(c)
	if err != nil {
		return err
	}
	if err = conn.VoiceMailDelete(c.Param("id")); err != nil {
		if _, ok := err.(*mx.CSTAError); ok {
			return rest.ErrNotFound
		}
		return err
	}
	return nil
}

// PatchVoiceMail изменяет заметку и/или флаг прочитанного голосового
// сообщения.
func (p *Proxy) PatchVoiceMail(c *rest.Context) error {
	conn, err := p.getConnection(c)
	if err != nil {
		return err
	}
	// разбираем переданные параметры
	var params = new(struct {
		Read *bool   `json:"read,omitempty" form:"read"`
		Note *string `json:"note,omitempty" form:"note"`
	})
	if err := c.Bind(params); err != nil {
		return err
	}
	// проверяем, что хотя бы один из них определен
	if params.Read == nil && params.Note == nil {
		return rest.ErrBadRequest
	}
	var msgID = c.Param("id")
	// изменяем текст заметки, если он задан
	if params.Read != nil {
		if err = conn.VoiceMailSetRead(msgID, *params.Read); err != nil {
			if _, ok := err.(*mx.CSTAError); ok {
				return rest.ErrNotFound
			}
			return err
		}
	}
	// изменяем отметку о прочтении, если она задана
	if params.Note != nil {
		if err = conn.VoiceMailSetNote(msgID, *params.Note); err != nil {
			if _, ok := err.(*mx.CSTAError); ok {
				return rest.ErrNotFound
			}
			return err
		}
	}
	return c.Write(rest.JSON{"vm": params})
}

// GetVoiceMailFile отдает содержимое файла с голосовым сообщением.
func (p *Proxy) GetVoiceMailFile(c *rest.Context) error {
	conn, err := p.getConnection(c)
	if err != nil {
		return err
	}
	// получаем информацию о файле с голосовой почтой
	vminfo, err := conn.VoiceMailFile(c.Param("id"))
	if err != nil {
		if _, ok := err.(*mx.CSTAError); ok {
			return rest.ErrNotFound
		}
		return err
	}
	// устанавливаем заголовки для ответа
	c.AddLogField("mime", vminfo.Mimetype)
	c.SetHeader("Content-Type", vminfo.Mimetype)
	c.SetHeader("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", vminfo.Name))
	// разрешаем отдавать ответ кусочками
	c.AllowMultiple = true
	// отслеживаем закрытие соединения пользователем
	var done = c.Request.Context().Done()
	for data := range vminfo.Chunks() {
		select {
		case <-done: // пользователь закрыл соединение
			vminfo.Cancel()                  // отменяем загрузку данных
			return c.Request.Context().Err() // возвращаем ошибку
		default: // отдаем кусочек данных пользователю
			if err = c.Write(data); err != nil {
				return err
			}
		}
	}
	return vminfo.Err() // все данные благополучно отосланы
}

// Token добавляет или удаляет токен из хранилища, в зависимости от метода
// запроса.
func (p *Proxy) Token(c *rest.Context) error {
	conn, err := p.getConnection(c)
	if err != nil {
		return err
	}
	var (
		tokenType = c.Param("type")  // тип токена: apn, fcm
		topicID   = c.Param("topic") // идентификатор приложения
		token     = c.Param("token") // токен устройства
	)
	// проверяем, что мы поддерживаем данные токены устройства
	switch tokenType {
	case "apn": // Apple Push Notification
		// проверяем, что взведен флаг sandbox
		if len(c.Request.URL.Query()["sandbox"]) > 0 {
			topicID += "~"
		}
		if !p.push.Support(tokenType, topicID) {
			return c.Error(http.StatusNotFound, "unsupported APNS topic ID or sandbox flag")
		}
	case "fcm": // Firebase Cloud Messages
		if !p.push.Support(tokenType, topicID) {
			return c.Error(http.StatusNotFound, "unsupported FCM application ID")
		}
	default:
		return c.Error(http.StatusNotFound,
			fmt.Sprintf("unsupported push type %q", tokenType))
	}
	if len(token) < 20 {
		return c.Error(http.StatusBadRequest, "bad push token")
	}
	switch c.Request.Method {
	case "POST", "PUT":
		return p.store.AddToken(tokenType, topicID, token, conn.Login)
	case "DELETE":
		return p.store.RemoveToken(tokenType, topicID, token)
	default:
		return rest.ErrMethodNotAllowed
	}
}

// Services возвращает список запущенных на MX сервисов.
func (p *Proxy) Services(c *rest.Context) error {
	conn, err := p.getConnection(c)
	if err != nil {
		return err
	}
	list, err := conn.GetServiceList()
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"services": list})
}
