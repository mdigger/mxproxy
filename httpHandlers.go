package main

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/mdigger/mx"
	"github.com/mdigger/rest"
)

// GetAddressBook отдает адресную книгу.
func (p *Proxy) GetAddressBook(c *rest.Context) error {
	mxs, err := p.getMXServer(c)
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"addressbook": mxs.AddressBook()})
}

// GetContact отдает информацию о пользователе с указанным идентификатором.
func (p *Proxy) GetContact(c *rest.Context) error {
	mxs, err := p.getMXServer(c)
	if err != nil {
		return err
	}
	// получаем информацию и отдаем информацию о пользователе
	if contact := mxs.Contact(c.Param("id")); contact != nil {
		return c.Write(rest.JSON{"contact": contact})
	}
	return rest.ErrNotFound // контакт не найден
}

// GetCallLog отдает лог звонков пользователя.
func (p *Proxy) GetCallLog(c *rest.Context) error {
	mxclient, err := p.getMXClient(c)
	if err != nil {
		return err
	}
	defer mxclient.Close()
	// разбираем параметра timestamp
	var ts time.Time
	var timestamp = c.Query("timestamp")
	if timestamp != "" {
		if t, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
			ts = time.Unix(t, 0)
		} else if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
			ts = t
		} else {
			return c.Error(http.StatusBadRequest, "bad timestamp format")
		}
	}
	// запрашиваем получения лога звонков
	list, err := mxclient.CallLog(ts)
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"calllog": list})
}

// PostMakeCall принимает запрос на инициализацию звонка и отправляет
// его на сервер MX.
func (p *Proxy) PostMakeCall(c *rest.Context) error {
	mxclient, err := p.getMXClient(c)
	if err != nil {
		return err
	}
	defer mxclient.Close()
	// Params описывает параметры, передаваемые в запроса
	type Params struct {
		RingDelay uint16 `xml:"ringdelay,attr" json:"ringDelay" form:"ringDelay"`
		VMDelay   uint16 `xml:"vmdelay,attr" json:"vmDelay" form:"vmDelay"`
		From      string `xml:"address" json:"from" form:"from"`
		To        string `xml:"-" json:"to" form:"to"`
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = &Params{
		RingDelay: 1,
		VMDelay:   30,
	}
	if err = c.Bind(params); err != nil {
		return err
	}
	// отправляем команды на сервер
	resp, err := mxclient.MakeCall(params.From, params.To,
		params.RingDelay, params.VMDelay)
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"call": resp}) // отдаем ответ
}

// PostSIPAnswer принимает запрос на подтверждение звонка и отправляет
// его на сервер MX.
func (p *Proxy) PostSIPAnswer(c *rest.Context) error {
	mxclient, err := p.getMXClient(c)
	if err != nil {
		return err
	}
	defer mxclient.Close()
	// Params описывает параметры, передаваемые в запроса
	callID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return rest.ErrNotFound
	}
	type Params struct {
		DeviceID string        `json:"deviceId" form:"deviceId"`
		SIPName  string        `json:"sipName" form:"sipName"`
		Timeout  time.Duration `json:"timeout" form:"timeout"`
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = new(Params)
	if err = c.Bind(params); err != nil {
		return err
	}
	// отправляем команды на сервер
	err = mxclient.SIPAnswer(callID, params.DeviceID,
		params.SIPName, params.Timeout*time.Second)
	if err == mx.ErrTimeout {
		return c.Error(http.StatusRequestTimeout, err.Error())
	} else if err == ErrNotAssigned {
		return c.Error(http.StatusBadRequest, err.Error())
	}
	return err
}

// GetVoiceMailList отдает лог с информацией о звонках пользователя.
func (p *Proxy) GetVoiceMailList(c *rest.Context) error {
	mxclient, err := p.getMXClient(c)
	if err != nil {
		return err
	}
	defer mxclient.Close()
	list, err := mxclient.VoiceMailList()
	if err != nil {
		return err
	}
	return c.Write(rest.JSON{"voicemails": list})
}

// GetVoiceMailFile отдает содержимое файла с голосовым сообщением.
func (p *Proxy) GetVoiceMailFile(c *rest.Context) error {
	mxclient, err := p.getMXClient(c)
	if err != nil {
		return err
	}
	defer mxclient.Close()
	// получаем информацию о файле с голосовой почтой
	vminfo, err := mxclient.VoiceMailFile(c.Param("id"))
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
			if err = c.Write(data); err != nil {
				return err
			}
		}
	}
	return vminfo.Err() // все данные благополучно отосланы
}

// DeleteVoiceMail удаляет голосовое сообщение пользователя.
func (p *Proxy) DeleteVoiceMail(c *rest.Context) error {
	mxclient, err := p.getMXClient(c)
	if err != nil {
		return err
	}
	defer mxclient.Close()
	return mxclient.VoiceMailDelete(c.Param("id"))
}

// PatchVoiceMail изменяет заметку и/или флаг прочитанности голосового сообщения.
func (p *Proxy) PatchVoiceMail(c *rest.Context) error {
	mxclient, err := p.getMXClient(c)
	if err != nil {
		return err
	}
	defer mxclient.Close()
	// разбираем переданные параметры
	var params = new(struct {
		Readed *bool   `json:"readed" form:"readed"`
		Note   *string `json:"note" form:"note"`
	})
	if err := c.Bind(params); err != nil {
		return err
	}
	// проверяем, что хотя бы один из них определен
	if params.Readed == nil && params.Note == nil {
		return rest.ErrBadRequest
	}

	var msgID = c.Param("id")
	// изменяем текст заметки, если он задан
	if params.Readed != nil {
		if err = mxclient.VoiceMailSetReaded(msgID, *params.Readed); err != nil {
			return err
		}
	}
	// изменяем отметку о прочтении, если она задана
	if params.Note != nil {
		return mxclient.VoiceMailSetNote(msgID, *params.Note)
	}
	return nil
}

// AddToken добавляет токен в хранилище.
func (p *Proxy) AddToken(c *rest.Context) error {
	mxs, err := p.getMXServer(c)
	if err != nil {
		return err
	}
	var (
		topicID   = c.Param("topic")       // идентификатор приложения
		tokenType = c.Param("type")        // тип токаена: apn, fcm
		jid       = c.Data("jid").(mx.JID) // идентификатор пользователя
	)
	// проверям, что мы поддерживаем данные токены устройства
	switch tokenType {
	case "apn": // Apple Push Notification
		// проверяем, что взведен флаг sandbox
		if c.Query("sandbox") != "" {
			topicID += "~"
		}
		if !p.apns.Support(topicID) {
			return c.Error(http.StatusNotFound, "unsupported APNS topic ID")
		}
	case "fcm": // Firebase Cloud Messages
		if _, ok := p.fcm[topicID]; !ok {
			return c.Error(http.StatusNotFound, "unsupported FCM application ID")
		}
	default:
		return c.Error(http.StatusNotFound,
			fmt.Sprintf("unsupported push type %q", tokenType))
	}
	// разбираем данные запроса
	var data = new(struct {
		Token string `json:"token" form:"token"`
	})
	if err := c.Bind(data); err != nil {
		return err
	}
	if len(data.Token) < 20 {
		return c.Error(http.StatusBadRequest, "bad push token")
	}
	// объединяем topicID c типом, а пользователся с идентификатором MX
	topicID = fmt.Sprintf("%s:%s", tokenType, topicID)
	var userID = fmt.Sprintf("%s:%s", mxs.ID(), jid)
	// поднимаем монитор для пользователя
	if err = mxs.MonitorStart(jid); err != nil {
		return err
	}
	// сохраняем токен в хранилище
	return p.store.TokenAdd(userID, topicID, data.Token)
}
