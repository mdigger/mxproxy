package main

import (
	"encoding/xml"
	"errors"
	"time"

	"github.com/mdigger/log"
)

// ErrNotAssigned возвращается при ошибке назначения устройства при ответе на
// SIP звонок.
var ErrNotAssigned = errors.New("not assigned")

// SIPAnswer подтверждает прием звонка по SIP.
func (c *MXClient) SIPAnswer(callID int64, deviceID, sipName string,
	timeout time.Duration) error {
	if _, err := c.conn.MonitorStart(""); err != nil {
		return err
	}
	c.log.WithFields(log.Fields{
		"callId":   callID,
		"deviceId": deviceID,
		"sipName":  sipName,
		"timeout":  timeout.Seconds(),
	}).Debug("sip answer")
	// отправляем команду для ассоциации устройства по имени
	type device struct {
		Type string `xml:"type,attr"`
		Name string `xml:",chardata"`
	}
	resp, err := c.conn.SendWithResponse(&struct {
		XMLName xml.Name `xml:"AssignDevice"`
		Device  device   `xml:"deviceID"`
	}{
		Device: device{
			Type: "device",
			Name: sipName,
		},
	})
	if err != nil {
		return err
	}
	// разбираем ответ
	var assign = new(struct {
		DeviceID string `xml:"AssignDeviceInfo>deviceID"`
	})
	if err = resp.Decode(assign); err != nil {
		return err
	}
	if assign.DeviceID != sipName {
		return ErrNotAssigned
	}
	// теперь отправляем команду на подтверждение звонка
	_, err = c.conn.SendWithResponseTimeout(&struct {
		XMLName  xml.Name `xml:"AnswerCall"`
		CallID   int64    `xml:"callToBeAnswered>callID"`
		DeviceID string   `xml:"callToBeAnswered>deviceID"`
	}{
		CallID:   callID,
		DeviceID: deviceID,
	}, timeout)
	return err
}

// MakeCall отсылает команду на сервер MX об установке соединения между двумя
// указанными телефонами.
func (c *MXClient) MakeCall(from, to string, ringDelay, vmDelay uint16) (
	*MakeCallResponse, error) {
	// отправляем команду на установку номера исходящего звонка
	if err := c.conn.Send(&struct {
		XMLName   xml.Name `xml:"iq"`
		Type      string   `xml:"type,attr"`
		ID        string   `xml:"id,attr"`
		Mode      string   `xml:"mode,attr"`
		RingDelay uint16   `xml:"ringdelay,attr"`
		VMDelay   uint16   `xml:"vmdelay,attr"`
		From      string   `xml:"address"`
	}{
		Type:      "set",
		ID:        "mode",
		Mode:      "remote",
		RingDelay: ringDelay,
		VMDelay:   vmDelay,
		From:      from,
	}); err != nil {
		return nil, err
	}
	// отправляем команду на звонок
	type callingDevice struct {
		Type string `xml:"typeOfNumber,attr"`
		Ext  string `xml:",chardata"`
	}
	var cmd = &struct {
		XMLName       xml.Name      `xml:"MakeCall"`
		CallingDevice callingDevice `xml:"callingDevice"`
		To            string        `xml:"calledDirectoryNumber"`
	}{
		CallingDevice: callingDevice{
			Type: "deviceID",
			Ext:  c.conn.Ext,
		},
		To: to,
	}
	resp, err := c.conn.SendWithResponse(cmd)
	if err != nil {
		return nil, err
	}
	var result = new(MakeCallResponse)
	if err = resp.Decode(result); err != nil {
		return nil, err
	}
	c.log.WithFields(log.Fields{
		"from": from,
		"to":   to,
	}).Debug("make call")
	return result, nil
}

// MakeCallResponse описывает информацию об ответе сервера MX об установке
// соединения между двумя телефонами.
type MakeCallResponse struct {
	CallID       int64  `xml:"callingDevice>callID" json:"callId"`
	DeviceID     string `xml:"callingDevice>deviceID" json:"deviceId"`
	CalledDevice string `xml:"calledDevice" json:"calledDevice"`
}
