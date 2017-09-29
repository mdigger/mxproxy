package main

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"sync"

	"github.com/mdigger/log"
	"github.com/mdigger/mx"
)

// Chunks описывает информацию о файле голосовой почты и предоставляет
// канал для его получения по частям.
type Chunks struct {
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

// Err возвращает описание ошибки.
func (c *Chunks) Err() error {
	c.mu.RLock()
	var err = c.err
	c.mu.RUnlock()
	return err
}

// Cancel отменяет получение содержимого файла и закрывает канал с данными.
func (c *Chunks) Cancel() (err error) {
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
func (c *Chunks) Chunks() <-chan []byte {
	// запускаем процедуру запросов и чтения ответов от сервера.
	go func() {
		ctxlog := log.With("id", c.ID)
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
			n, err := base64.StdEncoding.Decode(data, chunk.MediaContent)
			if err != nil {
				break
			}
			// log.Warn("chuck source", "len", len(data), "chunk", data)
			// проверяем, что получение данных не отменено
			select {
			case c.chunks <- data[:n]: // отсылаем данные клиенту
				ctxlog.Trace("voicemail chunk", "chunk",
					fmt.Sprintf("%02d/%02d", chunk.Number, chunk.Total))
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
