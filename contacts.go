package main

import (
	"sort"
	"sync"

	"github.com/mdigger/mx"
)

// Contact описывает информацию о пользователе из адресной книги.
type Contact struct {
	JID        mx.JID `json:"jid"`
	FirstName  string `json:"firstName"`
	LastName   string `json:"lastName"`
	Ext        string `json:"extension"`
	HomePhone  string `json:"homePhone,omitempty"`
	CellPhone  string `json:"cellPhone,omitempty"`
	Email      string `json:"email,omitempty"`
	HomeSystem mx.JID `json:"homeSystem,string,omitempty"`
	DID        string `json:"did,omitempty"`
	ExchangeID string `json:"exchangeId,omitempty"`
}

// Contacts содержит список контактов сервера MX.
type Contacts struct {
	contacts sync.Map
}

// Store сохраняет контакт в хранилище под его уникальным идентификатором.
func (c *Contacts) Store(contact *mx.Contact) {
	c.contacts.Store(contact.JID, &Contact{
		JID:        contact.JID,
		FirstName:  contact.FirstName,
		LastName:   contact.LastName,
		Ext:        contact.Ext,
		HomePhone:  contact.HomePhone,
		CellPhone:  contact.CellPhone,
		Email:      contact.Email,
		HomeSystem: contact.HomeSystem,
		DID:        contact.DID,
		ExchangeID: contact.ExchangeID,
	})
}

// Delete удаляет контакт с указанным идентификатором.
func (c *Contacts) Delete(jid mx.JID) {
	c.contacts.Delete(jid)
}

// List возвращает список контактов, упорядоченных по их внутренним номерам.
func (c *Contacts) List() []*Contact {
	var ab = make([]*Contact, 0)
	c.contacts.Range(func(jid, contact interface{}) bool {
		ab = append(ab, contact.(*Contact))
		return true
	})
	// сортируем по номерам телефонов
	sort.Slice(ab, func(i, j int) bool {
		return ab[i].Ext < ab[j].Ext
	})
	return ab
}

// Get возвращает контакт по его уникальному идентификатору.
func (c *Contacts) Get(jid mx.JID) *Contact {
	if contact, ok := c.contacts.Load(jid); ok {
		return contact.(*Contact)
	}
	return nil
}

// Find возвращает информацию о пользователе с указанным уникальным
// идентификатором, внутренним номером телефона или email-адресом.
func (c *Contacts) Find(id string) *Contact {
	if jid, err := mx.ParseJID(id); err == nil {
		if contact, ok := c.contacts.Load(jid); ok {
			return contact.(*Contact)
		}
	}
	var contact *Contact
	c.contacts.Range(func(_, data interface{}) bool {
		contact = data.(*Contact)
		return !(contact.Ext == id || contact.Email == id)
	})
	return contact
}
