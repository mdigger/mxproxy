package main

import (
	"encoding/xml"
	"net"
	"time"

	"github.com/mdigger/csta"
	"github.com/mdigger/log"
	"github.com/mdigger/rest"
)

// GetCallLog отдает пользовательский лог звонков.
func (mx *MX) GetCallLog(c *rest.Context) error {
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

	// разбираем параметра timestamp
	var ts int64 = -1
	timestamp := csta.ParseTimestamp(c.Query("timestamp"))
	if !timestamp.IsZero() {
		ts = timestamp.Unix()
	}
	if _, err := client.Send(&struct {
		XMLName   xml.Name `xml:"iq"`
		Type      string   `xml:"type,attr"`
		ID        string   `xml:"id,attr"`
		Timestamp int64    `xml:"timestamp,attr"`
	}{
		Type:      "get",
		ID:        "calllog",
		Timestamp: ts,
	}); err != nil {
		return err
	}
	client.SetWait(MXReadTimeout)
read:
	responce, err := client.Receive()
	if err != nil {
		// в случае таймаута возвращаем пустой лог, потому что нет другого
		// способа определить, что сервер не возвращает ответ
		if errNet, ok := err.(net.Error); ok && errNet.Timeout() {
			log.WithFields(log.Fields{
				"mx":       client.SN,
				"ext":      client.Ext,
				"count":    0,
				"fromTime": timestamp.Format(time.RFC3339),
			}).Debug("user empty call log")
			return nil
		}
		return err
	}
	if responce.Name != "callloginfo" { // игнорируем все ответы, кроме лога
		goto read
	}
	client.SetWait(0)
	var calllog = new(struct {
		LogItems []*MXCallInfo `xml:"callinfo"`
	})
	if err := responce.Decode(calllog); err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"mx":       client.SN,
		"ext":      client.Ext,
		"count":    len(calllog.LogItems),
		"fromTime": timestamp.Format(time.RFC3339),
	}).Debug("user call log")
	return c.Write(rest.JSON{"callog": calllog.LogItems})
}
