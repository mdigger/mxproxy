package main

import (
	"encoding/xml"
	"sort"
	"time"

	"github.com/mdigger/log"
	"github.com/mdigger/mx"
)

// CallLog возвращает информацию о звонках пользователя.
func (c *MXClient) CallLog(timestamp time.Time) ([]*CallInfo, error) {
	// формируем и отправляем команду получения лога звонков пользователя
	var ts int64
	if timestamp.IsZero() {
		ts = -1
	} else {
		ts = timestamp.Unix()
	}
	if err := c.conn.Send(&struct {
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
	err := c.conn.HandleWait(func(resp *mx.Response) error {
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
	c.log.WithFields(log.Fields{
		"count":     len(callLog),
		"timestamp": ts,
	}).Debug("calllog")
	return callLog, nil
}

// CallInfo описывает информацию о записи в логе звонков.
type CallInfo struct {
	Missed                bool   `xml:"missed,attr" json:"missed,omitempty"`
	Direction             string `xml:"direction,attr" json:"direction"`
	RecordID              int64  `xml:"record_id" json:"record_id"`
	GCID                  string `xml:"gcid" json:"gcid"`
	ConnectTimestamp      int64  `xml:"connectTimestamp" json:"connectTimestamp,omitempty"`
	DisconnectTimestamp   int64  `xml:"disconnectTimestamp" json:"disconnectTimestamp,omitempty"`
	CallingPartyNo        string `xml:"callingPartyNo" json:"callingPartyNo"`
	OriginalCalledPartyNo string `xml:"originalCalledPartyNo" json:"originalCalledPartyNo"`
	FirstName             string `xml:"firstName" json:"firstName,omitempty"`
	LastName              string `xml:"lastName" json:"lastName,omitempty"`
	Extension             string `xml:"extension" json:"extension,omitempty"`
	ServiceName           string `xml:"serviceName" json:"serviceName,omitempty"`
	ServiceExtension      string `xml:"serviceExtension" json:"serviceExtension,omitempty"`
	CallType              int64  `xml:"callType" json:"callType,omitempty"`
	LegType               int64  `xml:"legType" json:"legType,omitempty"`
	SelfLegType           int64  `xml:"selfLegType" json:"selfLegType,omitempty"`
	MonitorType           int64  `xml:"monitorType" json:"monitorType,omitempty"`
}
