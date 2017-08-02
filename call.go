package main

import (
	"encoding/xml"
	"fmt"

	"github.com/mdigger/rest"
)

// PostCall обрабатывает обратный вызов звонка.
func (mx *MX) PostCall(c *rest.Context) error {
	// проверяем авторизацию пользователя
	login, password, err := Authorize(c)
	if err != nil {
		return err
	}

	// Params описывает параметры, передаваемые в запроса
	type Params struct {
		RingDelay uint8  `xml:"ringdelay,attr" json:"ringDelay" form:"ringDelay"`
		VMDelay   uint8  `xml:"vmdelay,attr" json:"vmDelay" form:"vmDelay"`
		From      string `xml:"address" json:"from" form:"from"`
		To        string `xml:"-" json:"to" form:"to"`
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = &Params{
		RingDelay: 1,
		VMDelay:   30,
	}
	if err := c.Bind(params); err != nil {
		return err
	}
	// инициализируем пользовательское соединение с сервером MX
	client, err := mx.UserClient(login, password)
	if err != nil {
		return httpError(c, err)
	}
	defer client.Close()

	// отправляем команду на установку номера исходящего звонка
	if _, err = client.Send(&struct {
		XMLName xml.Name `xml:"iq"`
		Type    string   `xml:"type,attr"`
		ID      string   `xml:"id,attr"`
		Mode    string   `xml:"mode,attr"`
		*Params
	}{
		Type:   "set",
		ID:     "mode",
		Mode:   "remote",
		Params: params,
	}); err != nil {
		return err
	}
	// инициируем звонок на номер
	type callingDevice struct {
		Type string `xml:"typeOfNumber,attr"`
		Ext  string `xml:",chardata"`
	}
	var cmd = &struct {
		XMLName       xml.Name      `xml:"http://www.ecma-international.org/standards/ecma-323/csta/ed4 MakeCall"`
		CallingDevice callingDevice `xml:"callingDevice"`
		To            string        `xml:"calledDirectoryNumber"`
	}{
		CallingDevice: callingDevice{
			Type: "deviceID",
			Ext:  client.Ext,
		},
		To: params.To,
	}
	resp, err := client.SendWithResponse(cmd, MXReadTimeout)
	if err != nil {
		return err
	}
	switch resp.Name {
	case "CSTAErrorCode":
		cstaError := new(CSTAError)
		if err := resp.Decode(cstaError); err != nil {
			return err
		}
		return cstaError
	case "MakeCallResponse":
		var result = new(struct {
			CallID       uint64 `xml:"callingDevice>callID" json:"callId"`
			DeviceID     string `xml:"callingDevice>deviceID" json:"deviceId"`
			CalledDevice string `xml:"calledDevice" json:"called"`
		})
		if err := resp.Decode(result); err != nil {
			return err
		}
		return c.Write(result)
	default:
		return fmt.Errorf("unknown makecall response %s", resp.Name)
	}
}
