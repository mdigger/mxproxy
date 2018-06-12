package main

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"mime"
	"sort"
	"sync"
	"time"

	"github.com/mdigger/log"
	"github.com/mdigger/mx"
	"github.com/mdigger/rest"
)

func init() {
	// регистрируем mimetype для указанного расширения, чтобы IE корректно
	// мог его проигрывать, потому что стандартный тип для него - audio/x-wav.
	mime.AddExtensionType(".wav", "audio/wave")
}

// MXConn описывает пользовательское соединение с сервером MX.
type MXConn struct {
	Login     string // логин пользователя
	*MXConfig        // конфигурация для авторизации и подключения
	*mx.Conn         // соединение с сервером MX
	// monitorID int64    // идентификатор пользовательского монитора
	Calls sync.Map // текущие звонки
	Recs  sync.Map // информация о записанных звонках
}

// MXConnect устанавливает пользовательское соединение с сервером MX и
// авторизует пользователя. Строка с login используется исключительно для
// вывода в лог CSTA.
func MXConnect(conf *MXConfig, login string) (*MXConn, error) {
	conn, err := mx.Connect(conf.Host) // подключаемся к серверу MX
	if err != nil {
		return nil, err
	}
	// закрываем соединение при ошибке
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	// добавляем лог CSTA
	conn.SetLogger(log.New("mx:" + login))
	// авторизуемся
	if _, err = conn.Login(mx.Login{
		UserName:   conf.Login,
		Password:   conf.Password,
		Type:       "User", //"Mobile",
		Platform:   "iPhone",
		Version:    "7.0",
		LoginCapab: "Audio",
		MediaCapab: "Voicemail|CallRec",
	}); err != nil {
		return nil, err
	}
	// // отправляем команду на запуск монитора
	// resp, err := conn.SendWithResponse(&struct {
	// 	XMLName xml.Name `xml:"MonitorStart"`
	// 	Ext     string   `xml:"monitorObject>deviceObject"`
	// 	// ConfEvents bool     `xml:"confEvents"`
	// }{
	// 	Ext: conn.Ext,
	// 	// ConfEvents: true,
	// })
	// if err != nil {
	// 	return nil, err
	// }
	// // разбираем идентификатор монитора
	// var monitor = new(struct {
	// 	ID int64 `xml:"monitorCrossRefID"`
	// })
	// if err = resp.Decode(monitor); err != nil {
	// 	return nil, err
	// }

	return &MXConn{
		Login:    login,
		MXConfig: conf,
		Conn:     conn,
		// monitorID: monitor.ID,
	}, nil
}

// Close закрывает пользовательское соединение с сервером MX.
func (c *MXConn) Close() error {
	// // останавливаем пользовательский монитор
	// if c.monitorID != 0 {
	// 	if _, err := c.SendWithResponse(&struct {
	// 		XMLName xml.Name `xml:"MonitorStop"`
	// 		ID      int64    `xml:"monitorCrossRefID"`
	// 	}{
	// 		ID: c.monitorID,
	// 	}); err != nil {
	// 		return err
	// 	}
	// }
	// отправляем команду на деавторизацию
	if err := c.Logout(); err != nil {
		return err
	}
	return c.Conn.Close() // закрываем соединение
}

// Contacts возвращает список контактов сервера MX.
func (c *MXConn) Contacts() ([]*Contact, error) {
	// команда для запроса адресной книги
	var cmdGetAddressBook = &struct {
		XMLName xml.Name `xml:"iq"`
		Type    string   `xml:"type,attr"`
		ID      string   `xml:"id,attr"`
		Index   uint     `xml:"index,attr"`
	}{Type: "get", ID: "addressbook"}
	// отправляем запрос
	if err := c.Send(cmdGetAddressBook); err != nil {
		return nil, err
	}
	var contacts []*Contact // адресная книга
	//  инициализируем обработку ответов сервера
	if err := c.HandleWait(func(resp *mx.Response) error {
		// разбираем полученный кусок адресной книги
		var abList = new(struct {
			Size     uint       `xml:"size,attr" json:"size"`
			Index    uint       `xml:"index,attr" json:"index,omitempty"`
			Contacts []*Contact `xml:"abentry" json:"contacts,omitempty"`
		})
		if err := resp.Decode(abList); err != nil {
			return err
		}
		// инициализируем адресную книгу, если она еще не была инициализирована
		if contacts == nil {
			contacts = make([]*Contact, 0, abList.Size)
		}
		// заполняем адресную книгу полученными контактами
		contacts = append(contacts, abList.Contacts...)
		// проверяем, что получена вся адресная книга
		if (abList.Index+1)*50 < abList.Size {
			// увеличиваем номер для получения следующей "страницы" контактов
			cmdGetAddressBook.Index = abList.Index + 1
			// отправляем запрос на получение следующей порции
			return c.Send(cmdGetAddressBook)
		}
		return mx.Stop // заканчиваем обработку, т.к. все получили
	}, mx.ReadTimeout, "ablist"); err != nil {
		return nil, err
	}
	// сортируем по внутренним номерам
	sort.Slice(contacts, func(i, j int) bool {
		return contacts[i].Ext < contacts[j].Ext
	})
	return contacts, nil
}

// Contact описывает информацию о пользователе из адресной книги.
type Contact struct {
	JID        mx.JID `xml:"jid,attr" json:"jid,string"`
	FirstName  string `xml:"firstName" json:"firstName"`
	LastName   string `xml:"lastName" json:"lastName"`
	Ext        string `xml:"businessPhone" json:"ext"`
	HomePhone  string `xml:"homePhone" json:"homePhone,omitempty"`
	CellPhone  string `xml:"cellPhone" json:"cellPhone,omitempty"`
	Email      string `xml:"email" json:"email,omitempty"`
	HomeSystem mx.JID `xml:"homeSystem" json:"homeSystem,string,omitempty"`
	DID        string `xml:"did" json:"did,omitempty"`
	ExchangeID string `xml:"exchangeId" json:"exchangeId,omitempty"`
}

// CallLog возвращает информацию о звонках пользователя.
func (c *MXConn) CallLog(timestamp time.Time) ([]*CallInfo, error) {
	// формируем и отправляем команду получения лога звонков пользователя
	var ts int64
	if timestamp.IsZero() {
		ts = -1
	} else {
		ts = timestamp.Unix()
	}
	if err := c.Send(&struct {
		XMLName   xml.Name `xml:"iq"`
		Type      string   `xml:"type,attr"`
		ID        string   `xml:"id,attr"`
		Timestamp int64    `xml:"timestamp,attr"`
	}{
		Type:      "get",
		ID:        "calllog",
		Timestamp: ts,
	}); err != nil {
		return nil, err
	}

	// разбор ответов сервера
	var callLog []*CallInfo
	err := c.HandleWait(func(resp *mx.Response) error {
		var items = new(struct {
			LogItems []*CallInfo `xml:"callinfo"`
		})
		if err := resp.Decode(items); err != nil {
			return err
		}
		if callLog == nil {
			callLog = items.LogItems
		} else {
			callLog = append(callLog, items.LogItems...)
		}
		// BUG (d3): единственный способ, который я нашел для отслеживания
		// окончания лога звонков, это проверять количество звонков в ответе
		// блока - обычно блоки разбиты по 21.
		if len(items.LogItems) < 21 {
			return mx.Stop
		}
		return nil
	}, mx.ReadTimeout, "callloginfo")
	if err != nil && err != mx.ErrTimeout {
		return nil, err
	}
	// сортируем по номеру записи
	sort.Slice(callLog, func(i, j int) bool {
		return callLog[i].RecordID < callLog[j].RecordID
	})
	return callLog, nil
}

// CallInfo описывает информацию о записи в логе звонков.
type CallInfo struct {
	Missed                bool   `xml:"missed,attr" json:"missed"` // всегда отдавать
	Direction             string `xml:"direction,attr" json:"direction"`
	RecordID              int64  `xml:"record_id" json:"record_id"`
	GCID                  string `xml:"gcid" json:"gcid"`
	ConnectTimestamp      int64  `xml:"connectTimestamp" json:"connectTimestamp,omitempty"`
	DisconnectTimestamp   int64  `xml:"disconnectTimestamp" json:"disconnectTimestamp,omitempty"`
	CallingPartyNo        string `xml:"callingPartyNo" json:"callingPartyNo"`
	OriginalCalledPartyNo string `xml:"originalCalledPartyNo" json:"originalCalledPartyNo"`
	FirstName             string `xml:"firstName" json:"firstName,omitempty"`
	LastName              string `xml:"lastName" json:"lastName,omitempty"`
	Extension             string `xml:"extension" json:"ext,omitempty"`
	ServiceName           string `xml:"serviceName" json:"serviceName,omitempty"`
	ServiceExtension      string `xml:"serviceExtension" json:"serviceExtension,omitempty"`
	CallType              int64  `xml:"callType" json:"callType,omitempty"`
	LegType               int64  `xml:"legType" json:"legType,omitempty"`
	SelfLegType           int64  `xml:"selfLegType" json:"selfLegType,omitempty"`
	MonitorType           int64  `xml:"monitorType" json:"monitorType,omitempty"`
}

// AssignDevice ассоциирует телефонный номер с именем устройства.
func (c *MXConn) AssignDevice(name string) error {
	// отправляем команду для ассоциации устройства по имени
	type device struct {
		Type string `xml:"type,attr"`
		Name string `xml:",chardata"`
	}
	resp, err := c.SendAndWait(&struct {
		XMLName xml.Name `xml:"AssignDevice"`
		Device  device   `xml:"deviceID"`
	}{
		Device: device{
			Type: "device",
			Name: name,
		},
	}, "AssignDeviceInfo")
	if err != nil {
		return err
	}
	var assignDeviceInfo = new(struct {
		Name string `xml:"deviceID"`
	})
	if err := resp.Decode(assignDeviceInfo); err != nil {
		return err
	}
	if assignDeviceInfo.Name != name {
		return rest.NewError(400, fmt.Sprintf("bad device name %q", assignDeviceInfo.Name)) //ErrBadRequest
	}
	return nil
}

// SetCallMode устанавливает режим звонка.
func (c *MXConn) SetCallMode(remote bool, deviceID string, ringDelay, vmDelay uint16) error {
	var mode = "local"
	if remote {
		mode = "remote"
	}
	// отправляем команду на установку номера исходящего звонка
	return c.Send(&struct {
		XMLName   xml.Name `xml:"iq"`
		Type      string   `xml:"type,attr"`
		ID        string   `xml:"id,attr"`
		Mode      string   `xml:"mode,attr"`
		RingDelay uint16   `xml:"ringdelay,attr,omitempty"`
		VMDelay   uint16   `xml:"vmdelay,attr,omitempty"`
		From      string   `xml:"address,omitempty"`
	}{
		Type:      "set",
		ID:        "mode",
		Mode:      mode,
		RingDelay: ringDelay,
		VMDelay:   vmDelay,
		From:      deviceID,
	})
}

// MakeCall отсылает команду на сервер MX об установке соединения между двумя
// указанными телефонами.
func (c *MXConn) MakeCall(from, to, deviceID string) (*MakeCallResponse, error) {
	if deviceID != "" {
		// отправляем команду для ассоциации устройства по имени
		if err := c.AssignDevice(deviceID); err != nil {
			// if err != mx.ErrTimeout {
			return nil, err
			// }
		}
	}
	if from == "" {
		from = c.Ext
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
			Ext:  from,
		},
		To: to,
	}
	resp, err := c.SendWithResponse(cmd)
	if err != nil {
		return nil, err
	}
	var result = new(MakeCallResponse)
	if err = resp.Decode(result); err != nil {
		return nil, err
	}
	return result, nil
}

// MakeCallResponse описывает информацию об ответе сервера MX об установке
// соединения между двумя телефонами.
type MakeCallResponse struct {
	CallID       int64  `xml:"callingDevice>callID" json:"callId"`
	DeviceID     string `xml:"callingDevice>deviceID" json:"deviceId"`
	CalledDevice string `xml:"calledDevice" json:"calledDevice"`
}

// SIPAnswer подтверждает прием звонка по SIP.
func (c *MXConn) SIPAnswer(callID int64, deviceID string, assign bool, timeout time.Duration) error {
	if assign {
		// отправляем команду для ассоциации устройства по имени
		if err := c.AssignDevice(deviceID); err != nil {
			// if err != mx.ErrTimeout {
			return err
			// }
		}
	}
	// теперь отправляем команду на подтверждение звонка
	_, err := c.SendWithResponse(&struct {
		XMLName  xml.Name `xml:"AnswerCall"`
		CallID   int64    `xml:"callToBeAnswered>callID"`
		DeviceID string   `xml:"callToBeAnswered>deviceID"`
	}{
		CallID:   callID,
		DeviceID: deviceID,
	})
	return err
}

// Transfer перенаправляет звонок на другой номер.
func (c *MXConn) Transfer(callID int64, deviceID, to string) error {
	// теперь отправляем команду на подтверждение звонка
	_, err := c.SendWithResponse(&struct {
		XMLName  xml.Name `xml:"SingleStepTransferCall"`
		CallID   int64    `xml:"activeCall>callID"`
		DeviceID string   `xml:"activeCall>deviceID"`
		To       string   `xml:"transferredTo"`
	}{
		CallID:   callID,
		DeviceID: deviceID,
		To:       to,
	})
	return err
}

// ClearConnection сбрасывает звонок.
func (c *MXConn) ClearConnection(callID int64) (*ConnectionClearedEvent, error) {
	// теперь отправляем команду на подтверждение звонка
	resp, err := c.SendAndWait(&struct {
		XMLName  xml.Name `xml:"ClearConnection"`
		CallID   int64    `xml:"connectionToBeCleared>callID"`
		DeviceID string   `xml:"connectionToBeCleared>deviceID"`
	}{
		CallID:   callID,
		DeviceID: c.Ext,
	}, "ConnectionClearedEvent")
	if err != nil {
		return nil, err
	}
	var cleared = new(ConnectionClearedEvent)
	if resp != nil {
		if err := resp.Decode(cleared); err != nil {
			return nil, err
		}
		return cleared, nil
	}
	return nil, rest.NewError(400, "empty mx response")
}

// CallHold подвешивает звонок.
func (c *MXConn) CallHold(callID int64) (*HeldEvent, error) {
	resp, err := c.SendAndWait(&struct {
		XMLName  xml.Name `xml:"HoldCall"`
		CallID   int64    `xml:"callToBeHeld>callID"`
		DeviceID string   `xml:"callToBeHeld>deviceID"`
	}{
		CallID:   callID,
		DeviceID: c.Ext,
	}, "HeldEvent")
	if err != nil {
		return nil, err
	}
	var held = new(HeldEvent)
	if resp != nil {
		if err := resp.Decode(held); err != nil {
			return nil, err
		}
		return held, nil
	}
	return nil, rest.NewError(400, "empty mx response")
}

// CallUnHold разблокирует звонок.
func (c *MXConn) CallUnHold(callID int64) (*RetrievedEvent, error) {
	resp, err := c.SendAndWait(&struct {
		XMLName  xml.Name `xml:"RetrieveCall"`
		CallID   int64    `xml:"callToBeRetrieved>callID"`
		DeviceID string   `xml:"callToBeRetrieved>deviceID"`
	}{
		CallID:   callID,
		DeviceID: c.Ext,
	}, "RetrievedEvent")
	if err != nil {
		return nil, err
	}
	var retrived = new(RetrievedEvent)
	if resp != nil {
		if err := resp.Decode(retrived); err != nil {
			return nil, err
		}
		return retrived, nil
	}
	return nil, rest.NewError(400, "empty mx response")
}

// VoiceMailList возвращает список записей в голосовой почте пользователя.
func (c *MXConn) VoiceMailList( /*mediaType string*/ ) ([]*VoiceMail, error) {
	resp, err := c.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MailGetListIncoming"`
		UserID  string   `xml:"userID"`
		// MediaType string   `xml:"mediaType,omitempty"`
	}{
		UserID: c.Ext,
		// MediaType: mediaType,
	})
	if err != nil {
		return nil, err
	}
	var vmails = new(struct {
		Mails []*VoiceMail `xml:"mail" json:"voicemails"`
	})
	if err := resp.Decode(vmails); err != nil {
		return nil, err
	}
	for _, vm := range vmails.Mails {
		vm.MediaType = "VoiceMail"
	}
	return vmails.Mails, nil
}

// RecordsList возвращает список записанных звонков.
func (c *MXConn) RecordsList() []*VoiceMail {
	var recs = make([]*VoiceMail, 0)
	c.Recs.Range(func(_, value interface{}) bool {
		recs = append(recs, value.(*VoiceMail))
		return true
	})
	return recs
}

// VoiceMail описывает информацию о записи в голосовой почте.
type VoiceMail struct {
	From       string `xml:"from,attr" json:"from"`
	FromName   string `xml:"fromName,attr" json:"fromName,omitempty"`
	CallerName string `xml:"callerName,attr" json:"callerName,omitempty"`
	To         string `xml:"to,attr" json:"to"`
	OwnerType  string `xml:"ownerType,attr" json:"ownerType"`
	ID         string `xml:"mailId" json:"id"`
	MediaType  string `xml:"mediaType" json:"mediaType"`
	Received   int64  `xml:"received" json:"received"`
	Duration   uint16 `xml:"duration" json:"duration,omitempty"`
	Read       bool   `xml:"read" json:"read,omitempty"`
	Note       string `xml:"note" json:"note,omitempty"`
}

// VoiceMailDelete удаляет голосовое сообщение пользователя. При удалении
// голосового сообщения с несуществующим или чужим идентификатором ничего
// не происходит и ошибка не возвращается.
func (c *MXConn) VoiceMailDelete(id, mediaType string) error {
	_, err := c.SendWithResponse(&struct {
		XMLName   xml.Name `xml:"MailDeleteIncoming"`
		MailID    string   `xml:"mailId"`
		MediaType string   `xml:"mediaType,omitempty"`
	}{
		MailID:    id,
		MediaType: mediaType,
	})
	return err
}

// VoiceMailSetRead позволяет изменить состояние прочтения голосового сообщения.
func (c *MXConn) VoiceMailSetRead(id string, read bool, mediaType string) error {
	// ��апрашива��м первый кусочек файла с голосовым сообщением
	_, err := c.SendWithResponse(&struct {
		XMLName   xml.Name `xml:"MailSetStatus"`
		MailID    string   `xml:"mailId"`
		Flag      bool     `xml:"read"`
		MediaType string   `xml:"mediaType,omitempty"`
	}{
		MailID:    id,
		Flag:      read,
		MediaType: mediaType,
	})
	return err
}

// VoiceMailSetNote позволяет изменить комментарий голосового сообщения.
func (c *MXConn) VoiceMailSetNote(id, note, mediaType string) error {
	// запрашиваем первый кусочек файла с голосовым сообщением
	_, err := c.SendWithResponse(&struct {
		XMLName   xml.Name `xml:"UpdateVmNote"`
		MailID    string   `xml:"mailId"`
		Note      string   `xml:"note"`
		MediaType string   `xml:"mediaType,omitempty"`
	}{
		MailID:    id,
		Note:      note,
		MediaType: mediaType,
	})
	return err
}

// VoiceMailFile отдает содержимое файла с голосовым сообщением.
func (c *MXConn) VoiceMailFile(id, mediaType string) (*Chunks, error) {
	// запрашиваем первый кусочек файла с голосовым сообщением
	resp, err := c.SendWithResponse(&struct {
		XMLName   xml.Name `xml:"MailReceiveIncoming"`
		MailID    string   `xml:"faxSessionID"`
		MediaType string   `xml:"mediaType,omitempty"`
	}{
		MailID:    id,
		MediaType: mediaType,
	})
	if err != nil {
		return nil, err
	}
	// разбираем данные о первом куске
	var chunk = new(vmChunk)
	if err = resp.Decode(chunk); err != nil {
		return nil, err
	}
	// декодируем содержимое
	data, err := base64.StdEncoding.DecodeString(string(chunk.MediaContent))
	if err != nil {
		return nil, err
	}
	// формируем канал для передачи содержимого файла
	var chunks = make(chan []byte, 1)
	// и сразу отдаем в него содержимое первого куска файла, т.к. его уже получили
	chunks <- data
	// формируем строку с описанием типа содержимого
	var mimetype = mime.TypeByExtension("." + chunk.Format)
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	// создаем описание файла голосовой почты
	var vminfo = &Chunks{
		ID:        chunk.ID,    // идентификатор голосового сообщения
		Total:     chunk.Total, // общее количество кусков
		Mimetype:  mimetype,    // тип файла
		Name:      chunk.Name,  // название файла
		MediaType: mediaType,
		conn:      c.Conn,              // соединение с сервером
		chunks:    chunks,              // канал с содержимым файла
		done:      make(chan struct{}), // канал для закрытия
	}
	return vminfo, nil
}

// GetServiceList возвращает информацию о сервисах, запущенных на сервере MX.
func (c *MXConn) GetServiceList() ([]*MXServiceInfo, error) {
	resp, err := c.SendWithResponse("<GetServiceList/>")
	if err != nil {
		return nil, err
	}
	var services = new(struct {
		Services []*MXServiceInfo `xml:"Service"`
	})
	if err = resp.Decode(services); err != nil {
		return nil, err
	}
	return services.Services, nil
}

// MXServiceInfo описывает информацию о сервисе MX.
type MXServiceInfo struct {
	ID         mx.JID `xml:"serviceId" json:"id,string"`
	Name       string `xml:"serviceName" json:"name"`
	Type       string `xml:"serviceType" json:"type"`
	Ext        string `xml:"extension" json:"ext"`
	HomeSystem mx.JID `xml:"homeSystem" json:"homeSystem,string,omitempty"`
}

// Recording описывает информацию для записи звонков.
type Recording struct {
	CallID   int64  `xml:"Call>callID" json:"callID" form:"callID"`
	DeviceID string `xml:"Call>deviceID" json:"deviceID" form:"deviceID"`
	GroupID  string `xml:"groupID" json:"groupID" form:"groupID"`
}

// CallRecording инициализирует запись звонка.
func (c *MXConn) CallRecording(callID int64, deviceID, groupID string) error {
	if _, err := c.SendWithResponse(&struct {
		XMLName  xml.Name `xml:"StartRecording"`
		CallID   int64    `xml:"Call>callID"`
		DeviceID string   `xml:"Call>deviceID"`
		GroupID  string   `xml:"groupID,omitempty"`
	}{
		CallID:   callID,
		DeviceID: deviceID,
		GroupID:  groupID,
	}); err != nil {
		return err
	}
	return nil
}

// CallRecordingStop останавливает запись звонка.
func (c *MXConn) CallRecordingStop(callID int64, deviceID, groupID string) error {
	if _, err := c.SendWithResponse(&struct {
		XMLName  xml.Name `xml:"StopRecording"`
		CallID   int64    `xml:"Call>callID"`
		DeviceID string   `xml:"Call>deviceID"`
		GroupID  string   `xml:"groupID,omitempty"`
	}{
		CallID:   callID,
		DeviceID: deviceID,
		GroupID:  groupID,
	}); err != nil {
		return err
	}
	return nil
}
