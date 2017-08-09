package main

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"mime"

	"github.com/mdigger/log"
	"github.com/mdigger/rest"
)

// VoiceMailList отдает список голосовых сообщений.
func (mx *MX) VoiceMailList(c *rest.Context) error {
	// проверяем авторизацию пользователя
	login, password, err := Authorize(c)
	if err != nil {
		return err
	}
	// инициализируем пользовательское соединение с сервером MX
	client, err := mx.UserClient(login, password)
	if err != nil {
		return httpError(c, err)
	}
	defer client.Close()
	// отправляем команду на запуск монитора
	if _, err = client.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MonitorStart"`
		Ext     string   `xml:"monitorObject>deviceObject"`
	}{Ext: client.Ext}, 0); err != nil {
		return err
	}
	// запрашиваем и получаем список голосовых сообщений
	resp, err := client.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MailGetListIncoming"`
		UserID  string   `xml:"userID"`
	}{UserID: client.Ext}, 0)
	if err != nil {
		return err
	}
	var vmails = new(struct {
		Mails []*MXVoiceMail `xml:"mail" json:"voicemails"`
	})
	if err := resp.Decode(vmails); err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"mx":    client.SN,
		"ext":   client.Ext,
		"count": len(vmails.Mails),
	}).Debug("user voice mails")
	return c.Write(vmails)
}

func (mx *MX) GetVoiceMail(c *rest.Context) error {
	// проверяем авторизацию пользователя
	login, password, err := Authorize(c)
	if err != nil {
		return err
	}
	var mailID = c.Param("id")
	// инициализируем пользовательское соединение с сервером MX
	client, err := mx.UserClient(login, password)
	if err != nil {
		return httpError(c, err)
	}
	defer client.Close()

	// отправляем команду на запуск монитора
	if _, err = client.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MonitorStart"`
		Ext     string   `xml:"monitorObject>deviceObject"`
	}{Ext: client.Ext}, 0); err != nil {
		return err
	}
	// запрашиваем первый кусочек файла с голосовым сообщением
	resp, err := client.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MailReceiveIncoming"`
		MailID  string   `xml:"faxSessionID"`
	}{MailID: mailID}, 0)
	if err != nil {
		return err
	}
	// разрешаем отдавать ответ кусочками
	c.AllowMultiple = true
	// инициализируем получение информации о закрытии соединения HTTP клиентом
	closed := c.Request.Context().Done()
	ctxlog := log.WithFields(log.Fields{
		"ext": client.Ext,
		"mx":  client.SN,
	})
	var sended bool
next:
	// разбираем данные
	var chunk = new(MXVoiceMailChunk)
	if err := resp.Decode(chunk); err != nil {
		return err
	}
	// при получении первой порции устанавливаем заголовок с типом
	if !sended {
		sended = true
		mt := mime.TypeByExtension("." + chunk.Format)
		if mt == "" {
			mt = "application/octet-stream"
		}
		c.SetHeader("Content-Type", mt)
		c.SetHeader("Content-Disposition",
			fmt.Sprintf("attachment; filename=%q", chunk.DocName))
	}
	ctxlog = ctxlog.WithFields(log.Fields{
		"mailID":  chunk.MailId,
		"total":   chunk.Total,
		"current": chunk.Number,
	})
	// // отсылаем полученный кусочек данных пользователю в ответе на запрос
	// if flusher, ok := c.Response.(http.Flusher); ok {
	// 	flusher.Flush()
	// }
	select {
	case <-closed: // соединение закрыто пользователем
		ctxlog.Warning("voicemail chunk sending break")
		// прерываем получение голосового сообщения
		client.SendWithResponse(&struct {
			XMLName xml.Name `xml:"MailCancelReceive"`
			MailID  string   `xml:"mailId"`
		}{MailID: mailID}, 0)
		return nil
	default:
		// все хорошо, продолжаем
	}
	// декодируем содержимое и отдаем кусочек файла
	data, err := base64.StdEncoding.DecodeString(string(chunk.MediaContent))
	if err != nil {
		return err
	}
	if err := c.Write(data); err != nil {
		return err
	}
	// проверяем, что есть еще куски файла
	if chunk.Number >= chunk.Total {
		ctxlog.Debug("last voicemail chunk sended")
		return nil
	}
	ctxlog.Debug("voicemail chunk sended")
	// запрашиваем  следующий кусочек файла
	resp, err = client.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MailReceiveIncoming"`
		MailID  string   `xml:"faxSessionID"`
		Next    string   `xml:"nextChunk"`
	}{MailID: mailID}, 0)
	if err != nil {
		return err
	}
	goto next
}

func (mx *MX) DeleteVoiceMail(c *rest.Context) error {
	// проверяем авторизацию пользователя
	login, password, err := Authorize(c)
	if err != nil {
		return err
	}
	// инициализируем пользовательское соединение с сервером MX
	client, err := mx.UserClient(login, password)
	if err != nil {
		return httpError(c, err)
	}
	defer client.Close()

	// отправляем команду на запуск монитора
	if _, err = client.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MonitorStart"`
		Ext     string   `xml:"monitorObject>deviceObject"`
	}{Ext: client.Ext}, 0); err != nil {
		return err
	}
	// отправляем команду на удаление голосового сообщения
	_, err := client.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MailDeleteIncoming"`
		MailID  string   `xml:"mailId"`
	}{MailID: c.Param("id")}, 0)
	if err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"mx":     client.SN,
		"ext":    client.Ext,
		"mailId": msgID,
	}).Degug("voicemail deleted")
	return nil
}

func (mx *MX) PatchVoiceMail(c *rest.Context) error {
	// проверяем авторизацию пользователя
	login, password, err := Authorize(c)
	if err != nil {
		return err
	}
	msgID := c.Param("id")
	// разбираем переданные параметры
	var params = new(struct {
		Readed *bool   `json:"readed"`
		Note   *string `json:"note"`
	})
	if err := c.Bind(params); err != nil {
		return err
	}
	// проверяем, что хотя бы один из них определен
	if params.Readed == nil && params.Note == nil {
		return rest.ErrBadRequest
	}
	// инициализируем пользовательское соединение с сервером MX
	client, err := mx.UserClient(login, password)
	if err != nil {
		return httpError(c, err)
	}
	defer client.Close()

	ctxlog := log.WithFields(log.Fields{
		"mx":     client.SN,
		"ext":    client.Ext,
		"mailId": msgID,
	})
	// отправляем команду на запуск монитора
	if _, err = client.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MonitorStart"`
		Ext     string   `xml:"monitorObject>deviceObject"`
	}{Ext: client.Ext}, 0); err != nil {
		return err
	}
	// если есть флаг для пометки о прочтении, то ставим его
	if params.Readed != nil {
		// отправляем команду на удаление голосового сообщения
		_, err := client.SendWithResponse(&struct {
			XMLName xml.Name `xml:"MailSetStatus"`
			MailID  string   `xml:"mailId"`
			Flag    bool     `xml:"read"`
		}{MailID: msgID, Flag: *params.Readed}, 0)
		if err != nil {
			return err
		}
		ctxlog.WithField("readed", *params.Readed).Debug("voicemail readed flag set")
	}
	// если определен текст заметки, то присваиваем его голосовому сообщению
	if params.Note != nil {
		_, err = client.SendWithResponse(&struct {
			XMLName xml.Name `xml:"UpdateVmNote"`
			MailID  string   `xml:"mailId"`
			Note    string   `xml:"note"`
		}{MailID: msgID, Note: *params.Note}, 0)
		if err != nil {
			return err
		}
		ctxlog.WithField("note", *params.Note).Debug("voicemail note set")
	}

	return nil
}
