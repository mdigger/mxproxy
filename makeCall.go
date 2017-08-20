package main

import (
	"encoding/xml"

	"github.com/mdigger/log"
)

// MakeCall отсылает команду на сервер MX об установке соединения между двумя
// указанными телефонами.
func (s *MXClient) MakeCall(from, to string, ringDelay, vmDelay uint16) (
	*MakeCallResponse, error) {
	// отправляем команду на установку номера исходящего звонка
	if err := s.conn.Send(&struct {
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
			Ext:  s.conn.Ext,
		},
		To: to,
	}
	resp, err := s.conn.SendWithResponse(cmd)
	if err != nil {
		return nil, err
	}
	var result = new(MakeCallResponse)
	if err = resp.Decode(result); err != nil {
		return nil, err
	}
	s.log.WithFields(log.Fields{
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
	CalledDevice string `xml:"calledDevice" json:"called"`
}
