package main

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"mime"
	"sync"
	"time"

	"github.com/mdigger/log"
	"github.com/mdigger/mx"
)

func init() {
	// регистрируем mimetype для указанного расширения, чтобы IE корректно
	// мог его проигрывать, потому что стандартный тип для него - audio/x-wav.
	mime.AddExtensionType(".wav", "audio/wave")
}

// VoiceMailList возвращает список записей в голосовой почте пользователя.
func (c *MXClient) VoiceMailList() ([]*VoiceMail, error) {
	// отправляем команду на запуск монитора, если он еще не запущен
	// без нее эта штука не работает, хотя я и не понимаю почему
	if _, err := c.conn.MonitorStart(""); err != nil {
		return nil, err
	}
	// запрашиваем список голосовых сообщений
	resp, err := c.conn.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MailGetListIncoming"`
		UserID  string   `xml:"userID"`
	}{
		UserID: c.conn.Ext,
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
	c.log.WithField("count", len(vmails.Mails)).Debug("voicemail list")
	return vmails.Mails, nil
}

// VoiceMail описывает информацию о записи в голосовой почте.
type VoiceMail struct {
	From       string        `xml:"from,attr" json:"from"`
	FromName   string        `xml:"fromName,attr" json:"fromName,omitempty"`
	CallerName string        `xml:"callerName,attr" json:"callerName,omitempty"`
	To         string        `xml:"to,attr" json:"to"`
	OwnerType  string        `xml:"ownerType,attr" json:"ownerType"`
	ID         string        `xml:"mailId" json:"id"`
	Received   int64         `xml:"received" json:"received"`
	Duration   time.Duration `xml:"duration" json:"duration,omitempty"`
	Readed     bool          `xml:"read" json:"readed,omitempty"`
	Note       string        `xml:"note" json:"note,omitempty"`
}

// VoiceMailDelete удаляет голосовое сообщение пользователя. При удалении
// голосового сообщения с несуществующим или чужим идентификатором ничего
// не происходит и ошибка не возвращается.
func (c *MXClient) VoiceMailDelete(id string) error {
	// отправляем команду на запуск монитора, если он еще не запущен
	// без нее эта штука не работает, хотя я и не понимаю почему
	if _, err := c.conn.MonitorStart(""); err != nil {
		return err
	}
	// запрашиваем первый кусочек файла с голосовым сообщением
	_, err := c.conn.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MailDeleteIncoming"`
		MailID  string   `xml:"mailId"`
	}{
		MailID: id,
	})
	c.log.WithField("id", id).Debug("voicemail delete")
	return err
}

// VoiceMailSetReaded позволяет изменить состояние прочтения голосового сообщения.
func (c *MXClient) VoiceMailSetReaded(id string, readed bool) error {
	// отправляем команду на запуск монитора, если он еще не запущен
	// без нее эта штука не работает, хотя я и не понимаю почему
	if _, err := c.conn.MonitorStart(""); err != nil {
		return err
	}
	// запрашиваем первый кусочек файла с голосовым сообщением
	_, err := c.conn.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MailSetStatus"`
		MailID  string   `xml:"mailId"`
		Flag    bool     `xml:"read"`
	}{
		MailID: id,
		Flag:   readed,
	})
	c.log.WithFields(log.Fields{"id": id, "readed": readed}).
		Debug("voicemail flag")
	return err
}

// VoiceMailSetNote позволяет изменить комментарий голосового сообщения.
func (c *MXClient) VoiceMailSetNote(id string, note string) error {
	// отправляем команду на запуск монитора, если он еще не запущен
	// без нее эта штука не работает, хотя я и не понимаю почему
	if _, err := c.conn.MonitorStart(""); err != nil {
		return err
	}
	// запрашиваем первый кусочек файла с голосовым сообщением
	_, err := c.conn.SendWithResponse(&struct {
		XMLName xml.Name `xml:"UpdateVmNote"`
		MailID  string   `xml:"mailId"`
		Note    string   `xml:"note"`
	}{
		MailID: id,
		Note:   note,
	})
	c.log.WithField("id", id).Debug("voicemail note")
	return err
}

// VMChunks описывает информацию о файле голосовой почты и предоставляет
// канал для его получения по частям.
type VMChunks struct {
	ID       string `json:"id"`       // идентификатор сообщения
	Total    int    `json:"chunks"`   // общее количество частей
	Mimetype string `json:"mimeType"` // формат данных
	Name     string `json:"name"`     // название документа

	conn   *mx.Conn    // соединение с сервером
	chunks chan []byte // канал для передачи содержимого файла

	err error // описание ошибки, если она случилась
	mu  sync.RWMutex

	done chan struct{} // флаг закрытия канала
	once sync.Once     // закрытии выполняется только единожды
}

// VoiceMailFile возвращает информацию о файле с голосовым сообщением и канал
// для получения данных. Получение данных может быть отменено вызовом
// метода VMChunks.Cancel().
func (c *MXClient) VoiceMailFile(id string) (*VMChunks, error) {
	// отправляем команду на запуск монитора, если он еще не запущен
	// без нее эта штука не работает, хотя я и не понимаю почему
	if _, err := c.conn.MonitorStart(""); err != nil {
		return nil, err
	}
	// запрашиваем первый кусочек файла с голосовым сообщением
	resp, err := c.conn.SendWithResponse(&struct {
		XMLName xml.Name `xml:"MailReceiveIncoming"`
		MailID  string   `xml:"faxSessionID"`
	}{
		MailID: id,
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
	var vminfo = &VMChunks{
		ID:       chunk.ID,            // идентификатор голосового сообщения
		Total:    chunk.Total,         // общее количество кусков
		Mimetype: mimetype,            // тип файла
		Name:     chunk.Name,          // название файла
		conn:     c.conn,              // соединение с сервером
		chunks:   chunks,              // канал с содержимым файла
		done:     make(chan struct{}), // канал для закрытия
	}
	c.log.WithFields(log.Fields{
		"id":     chunk.ID,
		"chunk":  fmt.Sprintf("%02d/%02d", chunk.Number, chunk.Total),
		"format": chunk.Format,
	}).Debug("get voicemail file")
	return vminfo, nil
}

// Err возвращает описание ошибки.
func (c *VMChunks) Err() error {
	c.mu.RLock()
	var err = c.err
	c.mu.RUnlock()
	return err
}

// Cancel отменяет получение содержимого файла и закрывает канал с данными.
func (c *VMChunks) Cancel() (err error) {
	c.once.Do(func() {
		close(c.done) // закрываем канал окончания
		// посылаем команду для отмены на сервер MX
		// делаем это именно здесь, пока гарантировано не было закрыто
		// соединение с сервером MX, чего нельзя добиться в отдельном
		// потоке отсылки данных, а эта функция вызывается из основного
		// потока пользователя
		_, err = c.conn.SendWithResponse(&struct {
			XMLName xml.Name `xml:"MailCancelReceive"`
			MailID  string   `xml:"mailId"`
		}{
			MailID: c.ID,
		})
	})
	return
}

// Chunks возвращает канал для отдачи содержимого файла по частям.
func (c *VMChunks) Chunks() <-chan []byte {
	// запускаем процедуру запросов и чтения ответов от сервера.
	go func() {
		ctxlog := log.WithField("id", c.ID)
		var err error
	loop:
		for {
			// запрашиваем  следующий кусочек файла
			var resp *mx.Response
			resp, err = c.conn.SendWithResponse(&struct {
				XMLName xml.Name `xml:"MailReceiveIncoming"`
				MailID  string   `xml:"faxSessionID"`
				Next    string   `xml:"nextChunk"`
			}{
				MailID: c.ID,
			})
			if err != nil {
				break
			}
			// декодируем полученный кусок данных
			var chunk = new(vmChunk)
			if err = resp.Decode(chunk); err != nil {
				break
			}
			// декодируем содержимое
			var data = make([]byte, base64.StdEncoding.DecodedLen(
				len(chunk.MediaContent)))
			_, err = base64.StdEncoding.Decode(data, chunk.MediaContent)
			if err != nil {
				break
			}
			// проверяем, что получение данных не отменено
			select {
			case c.chunks <- data: // отсылаем данные клиенту
				ctxlog.WithField("chunk",
					fmt.Sprintf("%02d/%02d", chunk.Number, chunk.Total)).
					Debug("voicemail chunk")
				if chunk.Number >= c.Total {
					break loop // получили все куски данных
				}
			case <-c.done: // отмена передачи данных
				ctxlog.Debug("voicemail chunk canceled")
				break loop // прерываем работу
			}

		}
		c.mu.Lock()
		close(c.chunks) // закрываем канал по окончании
		c.err = err     // сохраняем ошибку
		c.mu.Unlock()
	}()
	return c.chunks // возвращаем канал
}

// vmChunk описывает кусок данных с файлом голосовой почты
type vmChunk struct {
	ID           string       `xml:"mailId,attr"`
	Number       int          `xml:"chunkNumber,attr"`
	Total        int          `xml:"totalChunks,attr"`
	Format       string       `xml:"fileFormat"`
	Name         string       `xml:"documentName"`
	MediaContent xml.CharData `xml:"mediaContent"`
}
