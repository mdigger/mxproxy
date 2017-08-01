package csta

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

var (
	// Максимальное время ожидания установки соединения с сервером.
	ConnectionTimeout = time.Second * 5
	// Максимальное время ожидания ответа от сервера
	ReadTimeout = time.Second * 5
	// Интервал для отправки keep-alive сообщений в случае простоя.
	KeepAliveDuration = time.Second * 60
)

// Client описывает соединение с сервером MX.
type Client struct {
	AuthInfo               // информация об авторизации
	conn      net.Conn     // сокетное соединение с сервером MX
	counter   uint16       // счетчик отосланных команд
	keepAlive *time.Timer  // таймер для отсылки keep-alive сообщений
	mu        sync.RWMutex // блокировка одновременного доступа
}

// NewClient устанавливает и возвращает соединение с сервером MX.
func NewClient(addr string, login Login) (*Client, error) {
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: ConnectionTimeout},
		"tcp", addr,
		&tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return nil, err
	}
	client := &Client{conn: conn}
	if err := client.login(login); err != nil {
		conn.Close()
		return nil, err
	}
	client.keepAlive = time.AfterFunc(KeepAliveDuration, client.sendKeepAlive)
	return client, nil
}

// sendKeepAlive отсылает на сервер keep-alive сообщение для поддержки активного
// соединения и взводит таймер для отправки следующего.
func (c *Client) sendKeepAlive() {
	if _, err := c.conn.Write([]byte{0x00, 0x00, 0x00, 0x15, 0x30, 0x30, 0x30,
		0x30, 0x3c, 0x6b, 0x65, 0x65, 0x70, 0x61, 0x6c, 0x69, 0x76, 0x65, 0x20,
		0x2f, 0x3e}); err == nil {
		c.mu.Lock()
		c.keepAlive.Reset(KeepAliveDuration)
		c.mu.Unlock()
		// c.log(false, 0, []byte("<keepalive/>"))
	}
}

// Close закрывает соединение с сервером.
func (c *Client) Close() error {
	c.keepAlive.Stop()
	c.log(false, 0, []byte("<close/>"))
	return c.conn.Close()
}

// buffers используется как пул буферов для формирования новых команд,
// отправляемых на сервер.
var buffers = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}

// Send отсылает команду на сервер. В качестве команды может выступать []byte,
// string или любой объект, который  может быть представлен в виде XML. Только в
// последнем случае происходит проверка валидности XML, поэтому используйте тип
// []byte или string исключительно в тех случаях, когда уверены в валидности
// содержимого этих команд. Пустые команды автоматически игнорируются.
// В ответ возвращается идентификатор отосланной команды или ошибка. Команды
// нумеруются от 1 до 9998: 0 зарезервирован за отправкой keep-alive сообщений,
// а 9999 используется сервером для асинхронных событий.
func (c *Client) Send(command interface{}) (id uint16, err error) {
	// преобразуем данные команды к формату XML
	var xmlData []byte
	switch data := command.(type) {
	case string:
		xmlData = []byte(data)
	case []byte:
		xmlData = data
	default:
		xmlData, err = xml.Marshal(command)
		if err != nil {
			return 0, err
		}
	}
	// проверяем, что есть данные для отправки
	if len(xmlData) == 0 {
		return 0, nil
	}
	// увеличиваем счетчик отправленных команд соединения,
	// при переполнении сбрасываем счетчик на 1
	c.mu.Lock()
	if c.counter > 9998 {
		c.counter = 1
	} else {
		c.counter++
	}
	id = c.counter
	// откладываем таймер отсылки keep-alive сообщений
	if c.keepAlive != nil {
		if !c.keepAlive.Stop() {
			<-c.keepAlive.C
		}
		c.keepAlive.Reset(KeepAliveDuration)
	}
	c.mu.Unlock()
	// формируем бинарное представление команды и отправляем ее
	var buf = buffers.Get().(*bytes.Buffer)
	buf.Write([]byte{0, 0})
	var length = uint16(len(xmlData) + len(xml.Header) + 8)
	binary.Write(buf, binary.BigEndian, length)
	fmt.Fprintf(buf, "%04d", id)
	io.WriteString(buf, xml.Header)
	buf.Write(xmlData)
	_, err = buf.WriteTo(c.conn)
	buffers.Put(buf)
	c.log(false, id, xmlData)
	return id, err
}

// Receive читает и возвращает ответ от сервера.
func (c *Client) Receive() (*Response, error) {
	var header = make([]byte, 8) // буфер для разбора заголовка ответа
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return nil, err
	}
	id, err := strconv.ParseUint(string(header[4:]), 10, 16)
	if err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint16(header[2:4]) - 8
	// читаем данные с ответом
	var data = make([]byte, length)
	if _, err = io.ReadFull(c.conn, data); err != nil {
		return nil, err
	}
	// разбираем ответ
	xmlDecoder := xml.NewDecoder(bytes.NewReader(data))
readToken:
	offset := xmlDecoder.InputOffset()
	token, err := xmlDecoder.Token()
	if err != nil {
		return nil, err
	}
	// пропускаем все до корневого элемента XML
	startToken, ok := token.(xml.StartElement)
	if !ok {
		goto readToken // игнорируем все до корневого элемента XML.
	}
	c.log(true, uint16(id), data[offset:])
	// отправляем разобранный ответ
	return &Response{
		ID:   uint16(id),            // идентификатор ответа
		Name: startToken.Name.Local, // название элемента
		data: data[offset:],         // неразобранные данные с ответом,
	}, nil
}

// SetWait устанавливает время ожидания ответа.
func (c *Client) SetWait(d time.Duration) error {
	if d <= 0 {
		return c.conn.SetReadDeadline(time.Time{})
	}
	return c.conn.SetReadDeadline(time.Now().Add(d))
}
