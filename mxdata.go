package main

import (
	"encoding/xml"
	"time"

	"github.com/mdigger/csta"
)

// CSTAError описывает ошибку CSTA.
type CSTAError struct {
	Message string `xml:",any"`
}

func (e *CSTAError) Error() string {
	return e.Message
}

// Contact описывает информацию о пользователе из адресной книги.
type Contact struct {
	JID csta.JID `xml:"jid,attr" json:"-"`
	// Presence   string   `xml:"presence,attr" json:"status,omitempty"`
	// Note       string   `xml:"presenceNote,attr" json:"note,omitempty"`
	FirstName  string   `xml:"firstName" json:"firstName"`
	LastName   string   `xml:"lastName" json:"lastName"`
	Ext        string   `xml:"businessPhone" json:"ext"`
	HomePhone  string   `xml:"homePhone" json:"homePhone,omitempty"`
	CellPhone  string   `xml:"cellPhone" json:"cellPhone,omitempty"`
	Email      string   `xml:"email" json:"email,omitempty"`
	HomeSystem csta.JID `xml:"homeSystem" json:"homeSystem,string,omitempty"`
	DID        string   `xml:"did" json:"did,omitempty"`
	ExchangeID string   `xml:"exchangeId" json:"exchangeId,omitempty"`
}

// MXCallInfo описывает информацию о записи в логе звонков.
type MXCallInfo struct {
	Missed                bool   `xml:"missed,attr" json:"missed,omitempty"`
	Direction             string `xml:"direction,attr" json:"direction"`
	RecordID              uint32 `xml:"record_id" json:"recordId"`
	GCID                  string `xml:"gcid" json:"gcid"`
	ConnectTimestamp      int64  `xml:"connectTimestamp" json:"connect,omitempty"`
	DisconnectTimestamp   int64  `xml:"disconnectTimestamp" json:"disconnect,omitempty"`
	CallingPartyNo        string `xml:"callingPartyNo" json:"callingPartyNo"`
	OriginalCalledPartyNo string `xml:"originalCalledPartyNo" json:"originalCalledPartyNo"`
	FirstName             string `xml:"firstName" json:"firstName,omitempty"`
	LastName              string `xml:"lastName" json:"lastName,omitempty"`
	Extension             string `xml:"extension" json:"ext,omitempty"`
	ServiceName           string `xml:"serviceName" json:"serviceName,omitempty"`
	ServiceExtension      string `xml:"serviceExtension" json:"serviceExtension,omitempty"`
	CallType              uint32 `xml:"callType" json:"callType,omitempty"`
	LegType               uint32 `xml:"legType" json:"legType,omitempty"`
	SelfLegType           uint32 `xml:"selfLegType" json:"selfLegType,omitempty"`
	MonitorType           uint32 `xml:"monitorType" json:"monitorType,omitempty"`
}

// Delivery описывает структуру события входящего звонка
type MXDelivery struct {
	MonitorCrossRefID     uint64 `xml:"monitorCrossRefID" json:"-"`
	CallID                uint64 `xml:"connection>callID" json:"callId"`
	DeviceID              string `xml:"connection>deviceID" json:"deviceId"`
	GlobalCallID          string `xml:"connection>globalCallID" json:"globalCallId"`
	AlertingDevice        string `xml:"alertingDevice>deviceIdentifier" json:"alertingDevice"`
	CallingDevice         string `xml:"callingDevice>deviceIdentifier" json:"callingDevice"`
	CalledDevice          string `xml:"calledDevice>deviceIdentifier" json:"calledDevice"`
	LastRedirectionDevice string `xml:"lastRedirectionDevice>deviceIdentifier" json:"lastRedirectionDevice"`
	LocalConnectionInfo   string `xml:"localConnectionInfo" json:"localConnectionInfo"`
	Cause                 string `xml:"cause" json:"cause"`
	CallTypeFlags         uint32 `xml:"callTypeFlags" json:"callTypeFlags,omitempty"`
	Timestamp             int64  `xml:"-" json:"time"`
}

type MXVoiceMail struct {
	From       string        `xml:"from,attr" json:"from"`
	FromName   string        `xml:"fromName,attr" json:"fromName,omitempty"`
	CallerName string        `xml:"callerName,attr" json:"callerName,omitempty"`
	To         string        `xml:"to,attr" json:"to"`
	OwnerType  string        `xml:"ownerType,attr" json:"ownerType"`
	MailId     string        `xml:"mailId" json:"id"`
	Received   int64         `xml:"received" json:"received"`
	Duration   time.Duration `xml:"duration" json:"duration,omitempty"`
	Read       bool          `xml:"read" json:"read,omitempty"`
	Note       string        `xml:"note" json:"note,omitempty"`
}

type MXVoiceMailChunk struct {
	MailId       string       `xml:"mailId,attr" json:"id"`
	Number       int          `xml:"chunkNumber,attr"`
	Total        int          `xml:"totalChunks,attr"`
	Format       string       `xml:"fileFormat"`
	DocName      string       `xml:"documentName"`
	MediaContent xml.CharData `xml:"mediaContent"`
}
