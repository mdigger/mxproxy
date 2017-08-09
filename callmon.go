package main

import (
	"encoding/xml"
	"sync"

	"github.com/mdigger/csta"
	"github.com/mdigger/log"
)

// CallMonsStart добавляет список идентификаторов пользователей для мониторинга
// входящих звонков.
func (mx *MX) CallMonsStart(jids ...csta.JID) error {
	ctxlog := log.WithField("mx", mx.SN)
	// команда для запуска монитора
	var cmd = new(struct {
		XMLName xml.Name `xml:"http://www.ecma-international.org/standards/ecma-323/csta/ed4 MonitorStart"`
		Ext     string   `xml:"monitorObject>deviceObject"`
	})
	for _, jid := range jids {
		if jid == 0 || mx.callmons.Exist(jid) {
			continue
		}
		// получаем информацию о контакте по его идентификатору
		mx.mu.RLock()
		contact := mx.contacts[jid]
		mx.mu.RUnlock()
		if contact == nil {
			ctxlog.WithField("jid", jid).Warning("ignore unknown monitor user")
			continue
		}
		// отправляем команды на запуск монитора
		cmd.Ext = contact.Ext
		ctxlog.WithFields(log.Fields{
			"jid": jid,
			"ext": contact.Ext,
		}).Debug("starting call monitor")
		id, err := mx.Send(cmd)
		if err != nil {
			return err
		}
		// сохраняем номер команды на старт монитора
		mx.callmons.Start(id, jid)
	}
	return nil
}

// CallMonsStop останавливает монитора для указанных пользователей, если они
// были запущены.
func (mx *MX) CallMonsStop(jids ...csta.JID) error {
	ctxlog := log.WithField("mx", mx.SN)
	var cmd = new(struct {
		XMLName xml.Name `xml:"http://www.ecma-international.org/standards/ecma-323/csta/ed4 MonitorStop"`
		ID      uint64   `xml:"monitorCrossRefID"`
	})
	for _, jid := range jids {
		if jid == 0 {
			continue
		}
		cmd.ID = mx.callmons.Stop(jid)
		if cmd.ID != 0 {
			continue
		}
		ctxlog.WithField("jid", jid).Debug("stoping call monitor")
		if _, err := mx.Send(cmd); err != nil {
			return err
		}
	}
	return nil
}

// CallMonitors описывает запущенные мониторы отслеживания входящих звонков.
type CallMonitors struct {
	commands map[uint16]csta.JID // номера команд на запуск монитора
	monitors map[uint64]csta.JID // запущенные мониторы
	mu       sync.RWMutex
}

// Start сохраняет информацию о команде на запуск монитора.
func (m *CallMonitors) Start(cmdID uint16, jid csta.JID) {
	m.mu.Lock()
	if m.commands == nil {
		m.commands = make(map[uint16]csta.JID)
	}
	m.commands[cmdID] = jid
	m.mu.Unlock()
}

// Started удаляет монитор из списка ожидания ответов и переводит его в
// список запущенных мониторов. В ответ возвращает идентификатор пользователя.
// Если идентификатор монитора 0, то он не добавляется в список запущенных.
func (m *CallMonitors) Started(cmdID uint16, monitorID uint64) {
	m.mu.Lock()
	if jid, ok := m.commands[cmdID]; ok {
		delete(m.commands, cmdID)
		if monitorID != 0 {
			if m.monitors == nil {
				m.monitors = make(map[uint64]csta.JID)
			}
			m.monitors[monitorID] = jid
		}
	}
	m.mu.Unlock()
}

// Stop удаляет монитор из списка запущенных. В ответ возвращается идентификатор
// монитора, если он был запущен.
func (m *CallMonitors) Stop(jid csta.JID) uint64 {
	m.mu.Lock()
	for monitorID, jid2 := range m.monitors {
		if jid2 != jid {
			continue
		}
		delete(m.monitors, monitorID)
		m.mu.Unlock()
		return monitorID
	}
	m.mu.Unlock()
	return 0
}

// JID возвращает уникальный идентификатор пользователя по номеру запущенного
// монитора. Если монитор с таким номером не запущен, то возвращается 0.
func (m *CallMonitors) JID(monitorID uint64) csta.JID {
	m.mu.RLock()
	jid := m.monitors[monitorID]
	m.mu.RUnlock()
	return jid
}

// Exist возвращает true, если входящие звонки данного пользователя
// уже мониторятся.
func (m *CallMonitors) Exist(jid csta.JID) bool {
	m.mu.RLock()
	for _, jid2 := range m.monitors {
		if jid2 != jid {
			continue
		}
		m.mu.RUnlock()
		return true
	}
	for _, jid2 := range m.commands {
		if jid2 != jid {
			continue
		}
		m.mu.RUnlock()
		return true
	}
	m.mu.RUnlock()
	return false
}

// Clear очищает все списки мониторов.
func (m *CallMonitors) Clear() {
	m.mu.Lock()
	m.commands = nil
	m.monitors = nil
	m.mu.Unlock()
}
