package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/mdigger/log"
	"github.com/mdigger/mx"
)

// MonitorStart запускает пользовательские мониторы для указанных пользователей.
// Не существующие пользователи "молча" игнорируются. Так что единственная
// ошибка, которая может возвратиться, это в случае проблем с соединением с MX.
func (s *MXServer) MonitorStart(jids ...mx.JID) error {
	for _, jid := range jids {
		var contact = s.contacts.Get(jid)
		if contact == nil {
			continue // неизвестный нам уникальный идентификатор пользователя
		}
		// запускаем пользовательский монитор
		id, err := s.conn.MonitorStart(contact.Ext)
		if err != nil {
			return err
		}
		// сохраняем идентификатор пользователя монитора
		s.monitors.Store(id, jid)
		s.log.WithFields(log.Fields{
			"jid":     jid,
			"ext":     contact.Ext,
			"monitor": id,
		}).Debug("user monitor started")
	}
	return nil
}

// MonitorStop останавливает мониторы для указанных пользователей.
// Если монитор пользователя с каким-то идентификатором не запущен, то не
// страшно - его отсечет обработчик остановки мониторов. Так что единственная
// ошибка, которая может вернуться, это ошибка соединения с MX.
func (s *MXServer) MonitorStop(jids ...mx.JID) (err error) {
	for _, jid := range jids {
		var id = s.monitors.Get(jid)
		if id == 0 {
			continue
		}
		s.monitors.Delete(id)
		if err = s.conn.MonitorStop(id); err != nil {
			return err
		}
		s.log.WithFields(log.Fields{
			"jid":     jid,
			"monitor": id,
		}).Debug("user monitor stoped")
	}
	return nil
}

// CallMonitor запускает мониторинг входящих звонков. Возвращает ошибку в
// случае ошибки соединения.
func (s *MXServer) CallMonitor(
	sendPush func(userID string, payload interface{}) error) error {
	s.log.Debug("call monitor started")
	var err = s.conn.Handle(func(resp *mx.Response) error {
		var delivery = new(Delivery)
		if err := resp.Decode(delivery); err != nil {
			return err
		}
		if delivery.CalledDevice == "" {
			return nil
		}
		delivery.Timestamp = time.Now().Unix()
		var jid = s.monitors.GetJID(delivery.MonitorCrossRefID)
		if jid == 0 {
			s.log.WithField("monitor", delivery.MonitorCrossRefID).
				Warning("monitor not found")
			return nil
		}
		s.log.WithFields(log.Fields{
			"monitor": delivery.MonitorCrossRefID,
			"callId":  delivery.CallID,
			"jid":     jid,
		}).Debug("incoming call")
		var userID = fmt.Sprintf("%s:%s", s.ID(), jid)
		// отправляем уведомление на устройства пользователя
		sendPush(userID, delivery)
		return nil
	}, "DeliveredEvent")
	s.log.Debug("call monitor stoped")
	return err
}

// Delivery описывает структуру события входящего звонка
type Delivery struct {
	MonitorCrossRefID     int64  `xml:"monitorCrossRefID" json:"-"`
	CallID                int64  `xml:"connection>callID" json:"callId"`
	DeviceID              string `xml:"connection>deviceID" json:"deviceId"`
	GlobalCallID          string `xml:"connection>globalCallID" json:"globalCallId"`
	AlertingDevice        string `xml:"alertingDevice>deviceIdentifier" json:"alertingDevice"`
	CallingDevice         string `xml:"callingDevice>deviceIdentifier" json:"callingDevice"`
	CalledDevice          string `xml:"calledDevice>deviceIdentifier" json:"calledDevice"`
	LastRedirectionDevice string `xml:"lastRedirectionDevice>deviceIdentifier" json:"lastRedirectionDevice"`
	LocalConnectionInfo   string `xml:"localConnectionInfo" json:"localConnectionInfo"`
	Cause                 string `xml:"cause" json:"cause"`
	CallTypeFlags         int32  `xml:"callTypeFlags" json:"callTypeFlags,omitempty"`
	Timestamp             int64  `xml:"-" json:"time"`
}

// Monitors хранит ассоциацию номера запущенного монитора пользователя и
// его уникального идентификатора.
type Monitors struct {
	monitors sync.Map
}

// Store регистрирует ассоциацию номера монитора и идентификатора пользователя.
func (m *Monitors) Store(id int64, jid mx.JID) {
	m.monitors.Store(id, jid)
}

// Delete удаляет запись по номеру монитора.
func (m *Monitors) Delete(id int64) {
	m.monitors.Delete(id)
}

// Get возвращает номер монитора по идентификатору пользователя. Если с
// пользователем не связан ни один монитор, то возвращается 0.
func (m *Monitors) Get(jid mx.JID) int64 {
	var monitor int64
	m.monitors.Range(func(id, jid2 interface{}) bool {
		monitor = id.(int64)
		return jid == jid2.(mx.JID)
	})
	return monitor
}

// GetJID возвращает уникальный идентификатор пользователя по номеру монитора.
// Если пользователь не привязан к монитору, то возвращается 0.
func (m *Monitors) GetJID(id int64) mx.JID {
	if jid, ok := m.monitors.Load(id); ok {
		return jid.(mx.JID)
	}
	return 0
}
