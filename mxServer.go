package main

import (
	"github.com/mdigger/log"
	"github.com/mdigger/mx"
)

// MXServer поддерживает серверное подключение к MX.
type MXServer struct {
	host      string       // адрес MX
	login     string       // серверный логин
	password  string       // пароль для авторизации
	conn      *mx.Conn     // соединение с сервером
	contacts  Contacts     // список контактов в адресной книге
	monitors  Monitors     // запущенные пользовательские мониторы и jid контактов
	authCache AuthCache    // кеш логинов пользователей
	log       *log.Context // для вывода лога
}

// ConnectMXServer устанавливает соединение с сервером и начинает работу.
func ConnectMXServer(host, login, password string) (*MXServer, error) {
	//устанавливаем соединение с сервером MX
	conn, err := mx.Connect(host, mx.Login{
		UserName: login,
		Password: password,
		Type:     "Server",
	})
	if err != nil {
		return nil, err
	}
	// инициализируем объект
	var mxServer = &MXServer{
		host:     host,
		login:    login,
		password: password,
		conn:     conn,
		log: log.WithFields(log.Fields{
			"mx":   conn.SN,
			"type": "server",
		}),
	}
	mxServer.log.WithFields(log.Fields{
		"login": login,
		"host":  host,
	}).Info("mx server connected")
	// получаем адресную книгу и запускаем ее мониторинг
	if err = mxServer.addressbook(); err != nil {
		mxServer.Close()
		return nil, err
	}
	return mxServer, nil
}

// addressbook запрашивает с сервера получение адресной книги и запускает
// моинторинг ее изменений.
func (s *MXServer) addressbook() error {
	ab, err := s.conn.Addressbook()
	if err != nil {
		return err
	}
	for _, contact := range ab {
		s.contacts.Store(contact)
	}
	s.log.WithField("contacts", len(ab)).Debug("address book received")

	// запускаем мониторинг изменений в адресной книге
	if _, err = s.conn.SendWithResponse("<MonitorStartAb/>"); err != nil {
		return err
	}
	s.log.Debug("address book monitor started")
	go s.conn.Handle(func(resp *mx.Response) error {
		switch resp.Name {
		case "AbUpdateUserEvent", "AbAddUserEvent":
			// добавление/изменения пользователя в адресной книге
			var update = new(struct {
				Contact *mx.Contact `xml:"abentry"`
			})
			if err := resp.Decode(update); err != nil {
				s.log.WithError(err).Errorf("mx event %s parse error", resp.Name)
				break
			}
			s.contacts.Store(update.Contact)
			s.log.WithField("jid", update.Contact.JID).Debug("contact updated")
		case "AbDeleteUserEvent":
			// удаление пользователя из адресной книги
			var update = new(struct {
				JID mx.JID `xml:"userId"`
			})
			if err := resp.Decode(update); err != nil {
				s.log.WithError(err).Errorf("mx event %s parse error", resp.Name)
				break
			}
			s.contacts.Delete(update.JID)
			s.log.WithField("jid", update.JID).Debug("contact deleted")
		}
		return nil
	}, "AbUpdateUserEvent", "AbAddUserEvent", "AbDeleteUserEvent")
	return nil
}

// Close закрывает соединение с сервером MX.
func (s *MXServer) Close() error {
	s.log.Info("mx server disconnected")
	return s.conn.Close()
}

// ID возвращает уникальный идентификатор MX.
func (s *MXServer) ID() string {
	return s.conn.SN
}

// ConnectMXClient устанавливает и возвращает пользовательское соединение с
// сервером MX.
func (s *MXServer) ConnectMXClient(login, password string) (*MXClient, error) {
	// устанавливаем пользовательское соединение с сервером MX.
	conn, err := ConnectMXClient(s.host, login, password)
	if err != nil {
		return nil, err
	}
	// добавляем в кеш информацию об авторизации пользователя.
	s.authCache.Add(login, password, conn.conn.JID)
	return conn, nil
}

// AddressBook возвращает адресную книгу с пользователями, упорядоченную по
// их уникальным идентификаторам.
func (s *MXServer) AddressBook() []*Contact {
	return s.contacts.List()
}

// Contact возвращает информацию о пользователе с указанным уникальным
// идентификатором, внутренним номером телефона или email-адресом.
func (s *MXServer) Contact(id string) *Contact {
	return s.contacts.Find(id)
}
