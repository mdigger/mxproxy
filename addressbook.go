package main

import (
	"encoding/xml"

	"github.com/mdigger/mxproxy/csta"
	"github.com/mdigger/rest"
)

// GetAddressBook отдает серверную адресную книгу.
func (mx *MX) GetAddressBook(c *rest.Context) error {
	// проверяем авторизацию пользователя
	login, password, err := Authorize(c)
	if err != nil {
		return err
	}
	// проверяем кеш с авторизацией
	if _, ok := mx.authCache.Check(login, password); !ok {
		// если в кеше нет записи, то авторизуем пользователя
		client, err := mx.UserClient(login, password)
		if err != nil {
			return httpError(c, err)
		}
		client.Close() // закрываем клиента в случае успешной авторизации
	}
	mx.mu.RLock()
	err = c.Write(rest.JSON{"addressbook": mx.contacts})
	mx.mu.RUnlock()
	return err
}

// GetAddressBook запрашивает и возвращает с сервера адресную книгу.
func GetAddressBook(client *csta.Client) (contacts map[csta.JID]*Contact, err error) {
	var cmd = &struct {
		XMLName xml.Name `xml:"iq"`
		Type    string   `xml:"type,attr"`
		ID      string   `xml:"id,attr"`
		Index   uint     `xml:"index,attr"`
	}{Type: "get", ID: "addressbook"}
send:
	if _, err := client.Send(cmd); err != nil {
		return nil, err
	}
read:
	client.SetWait(MXReadTimeout)     // устанавливаем время ожидания ответа
	responce, err := client.Receive() // читаем ответ от сервера
	if err != nil {
		return nil, err
	}
	if responce.Name != "ablist" { // игнорируем все, кроме адресной книги
		goto read
	}
	// разбираем полученный кусок адресной книги
	var abList = new(struct {
		Size     uint       `xml:"size,attr" json:"size"`
		Index    uint       `xml:"index,attr" json:"index,omitempty"`
		Contacts []*Contact `xml:"abentry" json:"contacts,omitempty"`
	})
	if err := responce.Decode(abList); err != nil {
		return nil, err
	}
	// инициализируем адресную книгу, если она еще не была инициализирована
	if contacts == nil {
		contacts = make(map[csta.JID]*Contact, abList.Size)
	}
	// заполняем адресную книгу полученными контактами
	for _, contact := range abList.Contacts {
		contacts[contact.JID] = contact
	}
	// проверяем, что получена вся адресная книга
	if (abList.Index+1)*50 < abList.Size {
		// увеличиваем номер для получения следующей "страницы" контактов
		cmd.Index = abList.Index + 1
		goto send
	}
	client.SetWait(0)    // сбрасываем время ожидания ответа
	return contacts, nil // возвращаем адресную книгу
}
