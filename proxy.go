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
	"github.com/mdigger/mx"
	"github.com/mdigger/rest"
)

// Proxy описывает сервис проксирования запросов к серверу MX.
type Proxy struct {
	adminWeb        string            // адрес административного сервера
	provisioningURL string            // адрес сервера провижининга
	appsAuth        map[string]string // связка client-id:secret
	store           *Store            // хранилище данных
	jwtGen          *JWTGenerator     // генератор авторизационных токенов
	conns           sync.Map          // пользовательские соединения с MX
	push            *Push             // отправитель уведомлений
	stopped         bool              // флаг остановки сервиса
	mu              sync.RWMutex
}

// InitProxy инициализирует и возвращает сервис проксирования запросов к MX.
func InitProxy() (proxy *Proxy, err error) {
	var config = &struct {
		ProvisioningURL string            `toml:"provisioning"`
		AppsAuth        map[string]string `toml:"apps"`
		DBName          string            `toml:"dbName"`
		LogName         string            `toml:"logName"`
		VoIP            struct {
			APNTTL string            `toml:"apnTTL"`
			APN    map[string]string `toml:"apn"`
			FCM    map[string]string `toml:"fcm"`
		} `toml:"voip"`
		JWT struct {
			TokenTTL   string `toml:"tokenTTL"`   // время жизни токена
			SingKeyTTL string `toml:"signKeyTTL"` // время жизни ключа
		} `toml:"jwt"`
		AdminWeb string `toml:"adminWeb"`
	}{
		ProvisioningURL: "https://config.connector73.net/config",
		DBName:          lowerAppName + ".db",
		AdminWeb:        "localhost:8043",
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
	if config.JWT.SingKeyTTL != "" {
		d, err := time.ParseDuration(config.JWT.SingKeyTTL)
		if err != nil {
			return nil, err
		}
		singKeyTTL = d
	}
	var jwtGen = NewJWTGenerator(tokenTTL, singKeyTTL)
	log.Info("token generator", "tokenTTL", tokenTTL, "signKeyTTL", singKeyTTL)

	// открываем хранилище
	store, err := OpenStore(config.DBName)
	if err != nil {
		log.Error("store error", "error", err)
		return nil, err
	}

	// загружаем сертификаты для VoIP Apple Push
	var push = &Push{
		store: store,
		apns:  make(map[string]*http.Client, len(config.VoIP.APN)),
		fcm:   config.VoIP.FCM,
	}
	// изменяем время жизни пуш-клиентов для APNS, если они указаны в конфиге
	if config.VoIP.APNTTL != "" {
		PushTimeout, err = time.ParseDuration(config.VoIP.APNTTL)
		if err != nil {
			return nil, err
		}
	}
	log.Info("apple push client idle", "timeout", PushTimeout)
	for filename, password := range config.VoIP.APN {
		if err := push.LoadCertificate(filename, password); err != nil {
			log.Error("apn certificate error", "filename", filename, "error", err)
		}
	}
	// выводим список поддерживаемых приложений для Firebase Cloud Messages
	for appName := range config.VoIP.FCM {
		log.Info("firebase cloud messaging", "app", appName)
	}
	// инициализируем прокси
	proxy = &Proxy{
		adminWeb:        config.AdminWeb,
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
			log.Error("mx user connection error", "login", login, "error", err)
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
		log.Error("mx user connection error", "error", err)
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
		// запускаем мониторинг звонков и голосовых сообщений
		err := conn.Handle(func(resp *mx.Response) error {
			switch resp.Name {
			case "DeliveredEvent": // входящий звонок
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
				delivered.Type = "Delivered"
				// сохраняем информацию о входящем звонке
				conn.Calls.Store(delivered.CallID, delivered)
				ctxlog.Debug("store call info", "id", delivered.CallID)
				p.push.Send(conn.Login, delivered) // отсылаем уведомление
				ctxlog.Info("incoming call", "id", delivered.CallID)
			case "EstablishedEvent": // состоявшийся звонок
				var established = new(EstablishedEvent)
				if err := resp.Decode(established); err != nil {
					return err
				}
				established.Timestamp = time.Now().Unix()
				established.Type = "Established"
				// сохраняем информацию о звонке
				conn.Calls.Store(established.CallID, established)
				ctxlog.Debug("store call info", "id", established.CallID)
				p.push.Send(conn.Login, established) // отсылаем уведомление
				ctxlog.Info("established call", "id", established.CallID)
			case "OriginatedEvent":
				var originated = new(OriginatedEvent)
				if err := resp.Decode(originated); err != nil {
					return err
				}
				originated.Timestamp = time.Now().Unix()
				originated.Type = "Originated"
				p.push.Send(conn.Login, originated) // отсылаем уведомление
				ctxlog.Info("originated call", "id", originated.CallID)
			case "ConnectionClearedEvent": // окончание звонка
				var cleared = new(ConnectionClearedEvent)
				if err := resp.Decode(cleared); err != nil {
					return err
				}
				// удаляем информацию о входящем звонке
				conn.Calls.Delete(cleared.CallID)
				ctxlog.Debug("delete call info", "id", cleared.CallID)
				cleared.Timestamp = time.Now().Unix()
				cleared.Type = "ConnectionCleared"
				p.push.Send(conn.Login, cleared) // отсылаем уведомление
				ctxlog.Info("connection cleared call", "id", cleared.CallID)
			case "HeldEvent": // блокировка звонка
				var held = new(HeldEvent)
				if err := resp.Decode(held); err != nil {
					return err
				}
				held.Timestamp = time.Now().Unix()
				held.Type = "HeldEvent"
				p.push.Send(conn.Login, held) // отсылаем уведомление
				ctxlog.Info("held call", "id", held.CallID)
			case "RetrievedEvent": // разблокировка звонка
				var retrived = new(RetrievedEvent)
				if err := resp.Decode(retrived); err != nil {
					return err
				}
				retrived.Timestamp = time.Now().Unix()
				retrived.Type = "RetrievedEvent"
				p.push.Send(conn.Login, retrived) // отсылаем уведомление
				ctxlog.Info("retrieved call", "id", retrived.CallID)
			case "MailIncomingReadyEvent": // новое голосовое сообщение
				var vmail = new(MailIncomingReadyEvent)
				if err := resp.Decode(vmail); err != nil {
					return err
				}
				// игнорируем прочитанные голосовые сообщения
				if vmail.Read {
					ctxlog.Debug("ignore readed voicemail", "id", vmail.MailID)
					return nil
				}
				vmail.Timestamp = time.Now().Unix()
				vmail.Type = "MailIncoming"
				p.push.Send(conn.Login, vmail) // отсылаем уведомление
				ctxlog.Info("new voice mail", "id", vmail.MailID)
			}
			return nil
		}, "DeliveredEvent", "MailIncomingReadyEvent", "EstablishedEvent",
			"OriginatedEvent", "ConnectionClearedEvent", "HeldEvent",
			"RetrievedEvent")
		// проверяем, что сервис или соединение не остановлены
		if _, ok := p.conns.Load(login); p.isStopped() || !ok {
			return // сервис или соединение остановлены
		}
		if err != nil {
			ctxlog.Error("monitoring error", "error", err)
		}
		// ждем окончания
		if err = <-conn.Done(); err != nil {
			ctxlog.Error("mx user connection error", "error", err)
		}
		p.conns.Delete(login) // удаляем из списка соединений
	reconnect:
		conf, err = p.store.GetUser(login) // получаем конфигураию
		if err != nil {
			ctxlog.Error("mx user config error", "error", err)
			return
		}
		ctxlog.Debug("mx user reconnecting", "delay", time.Minute)
		time.Sleep(time.Minute) // задержка перед переподключением
		if p.isStopped() {
			return // сервис остановлен
		}
		conn, err = MXConnect(conf, login)
		if err != nil {
			log.Error("mx user connection error", "error", err)
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
	Type                  string `xml:"-" json:"type"`
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

// EstablishedEvent описывает событие о состоявшемся звонке.
type EstablishedEvent struct {
	Type                  string `xml:"-" json:"type"`
	CallID                int64  `xml:"establishedConnection>callID" json:"callId"`
	DeviceID              string `xml:"establishedConnection>deviceID" json:"deviceId"`
	GlobalCallID          string `xml:"establishedConnection>globalCallID" json:"globalCallId"`
	AnsweringDevice       string `xml:"answeringDevice>deviceIdentifier" json:"answeringDevice"`
	AnsweringDisplayName  string `xml:"answeringDisplayName" json:"answeringDisplayName"`
	CallingDevice         string `xml:"callingDevice>deviceIdentifier" json:"callingDevice"`
	CalledDevice          string `xml:"calledDevice>deviceIdentifier" json:"calledDevice"`
	LastRedirectionDevice string `xml:"lastRedirectionDevice>deviceIdentifier" json:"lastRedirectionDevice,omitempty"`
	CallingDisplayName    string `xml:"callingDisplayName" json:"callingDisplayName"`
	Cause                 string `xml:"cause" json:"cause"`
	CallTypeFlags         uint32 `xml:"callTypeFlags" json:"callTypeFlags,omitempty"`
	CmdsAllowed           uint32 `xml:"cmdsAllowed" json:"cmdsAllowed,omitempty"`
	Cads                  []struct {
		Name  string `xml:"name,attr" json:"name"`
		Type  string `xml:"type,attr" json:"type"`
		Value string `xml:",chardata" json:"value,omitempty"`
	} `xml:"cad,omitempty" json:"cads,omitempty"`
	Timestamp int64 `xml:"-" json:"timestamp"`
}

// OriginatedEvent описывает собитие о входящем звонке.
type OriginatedEvent struct {
	Type          string `xml:"-" json:"type"`
	CallID        int64  `xml:"originatedConnection>callID" json:"callId"`
	DeviceID      string `xml:"originatedConnection>deviceID" json:"deviceId"`
	CallingDevice string `xml:"callingDevice>deviceIdentifier" json:"callingDevice"`
	CalledDevice  string `xml:"calledDevice>deviceIdentifier" json:"calledDevice"`
	Cause         string `xml:"cause" json:"cause"`
	CallTypeFlags uint32 `xml:"callTypeFlags" json:"callTypeFlags,omitempty"`
	CmdsAllowed   uint32 `xml:"cmdsAllowed" json:"cmdsAllowed,omitempty"`
	Timestamp     int64  `xml:"-" json:"timestamp"`
}

// ConnectionClearedEvent описывает событие о завершенном звонке.
type ConnectionClearedEvent struct {
	Type            string `xml:"-" json:"type"`
	CallID          int64  `xml:"droppedConnection>callID" json:"callId"`
	DeviceID        string `xml:"droppedConnection>deviceID" json:"deviceId"`
	ReleasingDevice string `xml:"releasingDevice>deviceIdentifier" json:"releasingDevice"`
	Cause           string `xml:"cause" json:"cause"`
	Timestamp       int64  `xml:"-" json:"timestamp"`
}

// HeldEvent описывает событие о блокировке звонка.
type HeldEvent struct {
	Type            string `xml:"-" json:"type"`
	CallID          int64  `xml:"heldConnection>callID" json:"callId"`
	DeviceID        string `xml:"heldConnection>deviceID" json:"deviceId"`
	ReleasingDevice string `xml:"holdingDevice>deviceIdentifier" json:"releasingDevice"`
	Cause           string `xml:"cause" json:"cause"`
	CallTypeFlags   uint32 `xml:"callTypeFlags" json:"callTypeFlags,omitempty"`
	CmdsAllowed     uint32 `xml:"cmdsAllowed" json:"cmdsAllowed,omitempty"`
	Timestamp       int64  `xml:"-" json:"timestamp"`
}

// RetrievedEvent описывает событие о разблокировке звонка.
type RetrievedEvent struct {
	Type            string `xml:"-" json:"type"`
	CallID          int64  `xml:"retrievedConnection>callID" json:"callId"`
	DeviceID        string `xml:"retrievedConnection>deviceID" json:"deviceId"`
	ReleasingDevice string `xml:"retrievingDevice>deviceIdentifier" json:"releasingDevice"`
	Cause           string `xml:"cause" json:"cause"`
	CallTypeFlags   uint32 `xml:"callTypeFlags" json:"callTypeFlags,omitempty"`
	CmdsAllowed     uint32 `xml:"cmdsAllowed" json:"cmdsAllowed,omitempty"`
	Timestamp       int64  `xml:"-" json:"timestamp"`
}

// MailIncomingReadyEvent описывает структуру о новом голосовом сообщении
type MailIncomingReadyEvent struct {
	Type       string `xml:"-" json:"type"`
	From       string `xml:"from,attr" json:"from"`
	FromName   string `xml:"fromName,attr" json:"fromName"`
	CallerName string `xml:"callerName,attr" json:"callerName"`
	To         string `xml:"to,attr" json:"to"`
	// Private           string `xml:"private,attr" json:"private"`
	// Urgent            string `xml:"urgent,attr" json:"urgent"`
	OwnerID           int64  `xml:"ownerId" json:"ownerId"`
	OwnerType         string `xml:"ownerType,attr" json:"ownerType"`
	MonitorCrossRefID int64  `xml:"monitorCrossRefID" json:"-"`
	MailID            string `xml:"mailId" json:"mailId"`
	// MediaType         string `xml:"mediaType" json:"mediaType"`
	GlobalCallID string `xml:"gcid" json:"gcid"`
	Received     int64  `xml:"received" json:"received"`
	Duration     uint16 `xml:"Duration" json:"duration"`
	Read         bool   `xml:"read" json:"-"`
	// FileFormat        string `xml:"fileFormat" json:"fileFormat"`
	// Note              string `xml:"note" json:"note,omitempty"`
	Timestamp int64 `xml:"-" json:"timestamp"`
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
	// проверяем авторизацию на сервере провижининга
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
	log.Info("mx user disconnected", "login", login)
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
		MX           string `json:"mx"`
		Ext          string `json:"ext"`
		JID          mx.JID `json:"jid,string"`
		SoftPhonePwd string `json:"softPhonePwd"`
	}{
		MX:           conn.SN,
		Ext:          conn.Ext,
		JID:          conn.JID,
		SoftPhonePwd: conn.SoftPhonePwd,
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
	if err = conn.SetCallMode(params.Remote, params.Device,
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
		return c.Error(http.StatusNotFound, err.Error())
	}
	// инициализируем параметры по умолчанию и разб��ра��м запр����с
	var params = &struct {
		CallID  int64  `json:"callId" form:"callId"`
		Device  string `json:"device" form:"device"`
		Timeout uint16 `json:"timeout" form:"timeout"`
		Assign  bool   `json:"assign" form:"assign"`
	}{
		Timeout: 30,
	}
	if err = c.Bind(params); err != nil {
		return err
	}
	if err = conn.SIPAnswer(callID, params.Device, params.Assign,
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
		return c.Error(http.StatusNotFound, err.Error())
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

// ClearConnection сбрасывает звонок.
func (p *Proxy) ClearConnection(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	callID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.Error(http.StatusNotFound, err.Error())
	}
	cleared, err := conn.ClearConnection(callID)
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"connectionCleared": cleared})
}

// CallHold блокирует звонок.
func (p *Proxy) CallHold(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	callID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.Error(http.StatusNotFound, err.Error())
	}
	held, err := conn.CallHold(callID)
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"held": held})
}

// CallUnHold разблокирует звонок.
func (p *Proxy) CallUnHold(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	callID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.Error(http.StatusNotFound, err.Error())
	}
	retrieved, err := conn.CallUnHold(callID)
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"retrieved": retrieved})
}

// CallInfo отдает информацию о звонке.
func (p *Proxy) CallInfo(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	callID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.Error(http.StatusNotFound, err.Error())
	}
	callInfo, ok := conn.Calls.Load(callID)
	if !ok {
		return rest.ErrNotFound
	}
	return c.Write(rest.JSON{"callInfo": callInfo})
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
			// log.Warn("chuck destination", "len", len(data), "chunk", data)
			if err = c.Write(data); err != nil {
				return err
			}
			// c.Write([]byte("\n\n-----\n\n"))
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

// ConferenceList возвращает список конференций.
func (p *Proxy) ConferenceList(c *rest.Context) error {
	conn, err := p.getConnection(c)
	if err != nil {
		return err
	}
	list, err := conn.ConferenceList()
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"conferences": list})
}

// ConferenceDelete удаляет конференцию.
func (p *Proxy) ConferenceDelete(c *rest.Context) error {
	conn, err := p.getConnection(c)
	if err != nil {
		return err
	}
	if err = conn.ConferenceDelete(c.Param("id")); err != nil {
		if _, ok := err.(*mx.CSTAError); ok {
			return rest.ErrNotFound
		}
		return err
	}
	return nil
}

// ConferenceCreate создает новую конференцию.
func (p *Proxy) ConferenceCreate(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = new(Conference)
	if err = c.Bind(params); err != nil {
		return err
	}
	if params.OwnerID == 0 {
		params.OwnerID = conn.JID
	}
	params, err = conn.ConferenceCreate(params)
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"conference": params})
}

// ConferenceUpdate изменяет информацю о конференции.
func (p *Proxy) ConferenceUpdate(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = new(Conference)
	if err = c.Bind(params); err != nil {
		return err
	}
	params.ID = c.Param("id") // подменяем идентификатор
	if params.OwnerID == 0 {
		params.OwnerID = conn.JID
	}
	params, err = conn.ConferenceUpdate(params)
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"conference": params})
}

// ConferenceInfo возвращает настройки конференций.
func (p *Proxy) ConferenceInfo(c *rest.Context) error {
	conn, err := p.getConnection(c)
	if err != nil {
		return err
	}
	list, err := conn.ConferenceServerInfo()
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"conferences": list})
}

// ConferenceJoin позволяет присоединиться к конференции.
func (p *Proxy) ConferenceJoin(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = new(struct {
		AccessID int64 `json:"accessId" form:"accessId"`
	})
	if err = c.Bind(params); err != nil {
		return err
	}
	ID := c.Param("id")
	return conn.ConferenceJoin(ID, params.AccessID)
}

// ConferenceCreateFromCall создает конференцию из звонка.
func (p *Proxy) ConferenceCreateFromCall(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = new(struct {
		OwnerCallID int64 `json:"ownerCallId" form:"ownerCallId"`
	})
	if err = c.Bind(params); err != nil {
		return err
	}
	callID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.Error(http.StatusNotFound, err.Error())
	}
	return conn.ConferenceCreateFromCall(callID, params.OwnerCallID)
}

// CallRecording инициализирует запись звонка.
func (p *Proxy) CallRecording(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = new(struct {
		DeviceID string `json:"deviceID" form:"deviceID"`
		GroupID  string `json:"groupID" form:"groupID"`
	})
	if err = c.Bind(params); err != nil {
		return err
	}
	callID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.Error(http.StatusNotFound, err.Error())
	}
	return conn.CallRecording(callID, params.DeviceID, params.GroupID)
}

// CallRecordingStop останавливает запись звонка.
func (p *Proxy) CallRecordingStop(c *rest.Context) error {
	conn, err := p.getConnection(c) // проверяем токен и получаем соединение
	if err != nil {
		return err
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = new(struct {
		DeviceID string `json:"deviceID" form:"deviceID"`
		GroupID  string `json:"groupID" form:"groupID"`
	})
	if err = c.Bind(params); err != nil {
		return err
	}
	callID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.Error(http.StatusNotFound, err.Error())
	}
	return conn.CallRecordingStop(callID, params.DeviceID, params.GroupID)
}
