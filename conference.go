package main

import (
	"encoding/xml"

	"github.com/mdigger/mx"
)

// Conference описывает параметры конференции.
type Conference struct {
	ID              string `xml:"confId" json:"Id"`
	OwnerID         mx.JID `xml:"ownerId" json:"ownerId"`
	Name            string `xml:"name" json:"name"`
	AccessID        int64  `xml:"accessId" json:"accessId"`
	Description     string `xml:"description" json:"description,omitempty"`
	Type            string `xml:"type" json:"type"`
	StartDate       int64  `xml:"startDate" json:"startDate"`
	Duration        int64  `xml:"duration" json:"duration"`
	WaitForOwner    bool   `xml:"waitForOwner" json:"waitForOwner"`
	DelOnOwnerLeave bool   `xml:"delOnOwnerLeave" json:"delOnOwnerLeave"`
	Ws              string `xml:"ws" json:"ws"`
	WsType          string `xml:"wsType" json:"wsType"`
}

// ConferenceCreate инициализирует новую конференцию.
func (c *MXConn) ConferenceCreate(params *Conference) (*Conference, error) {
	if _, err := c.SendWithResponse(&struct {
		XMLName xml.Name `xml:"CreateConference"`
		*Conference
	}{
		Conference: params,
	}); err != nil {
		return nil, err
	}
	err := c.HandleWait(func(resp *mx.Response) error {
		if err := resp.Decode(params); err != nil {
			return err
		}
		return mx.Stop
	}, mx.ReadTimeout, "ConfAddEvent")
	if err != nil {
		return nil, err
	}
	return params, nil
}

// ConferenceUpdate изменяет информацю о конференции.
func (c *MXConn) ConferenceUpdate(params *Conference) (*Conference, error) {
	if _, err := c.SendWithResponse(&struct {
		XMLName xml.Name `xml:"UpdateConference"`
		*Conference
	}{
		Conference: params,
	}); err != nil {
		return nil, err
	}
	err := c.HandleWait(func(resp *mx.Response) error {
		if err := resp.Decode(params); err != nil {
			return err
		}
		return mx.Stop
	}, mx.ReadTimeout, "ConfUpdEvent")
	if err != nil {
		return nil, err
	}
	return params, nil
}

// ConferenceDelete удаляет конфиренцию.
func (c *MXConn) ConferenceDelete(id string) error {
	if _, err := c.SendWithResponse(&struct {
		XMLName xml.Name `xml:"DeleteConference"`
		ID      string   `xml:"confId"`
	}{
		ID: id,
	}); err != nil {
		return err
	}
	return c.HandleWait(func(resp *mx.Response) error {
		return mx.Stop
	}, mx.ReadTimeout, "ConfDelEvent")
}

// ServerConferenceInfo описывает информацию с ответом о серверной конференции.
type ServerConferenceInfo struct {
	Ext     string `xml:"extension" json:"ext"`
	DID     string `xml:"DID" json:"did"`
	Subject string `xml:"inviteSubject" json:"subject"`
	Body    string `xml:"inviteBody" json:"body"`
	Meeting string `xml:"inviteMXmeeting" json:"meeting"`
}

// ConferenceServerInfo возвращает серверную информацию для создания конференции.
func (c *MXConn) ConferenceServerInfo() (*ServerConferenceInfo, error) {
	resp, err := c.SendWithResponse("<GetConfServerInfo/>")
	if err != nil {
		return nil, err
	}
	var result = new(ServerConferenceInfo)
	if err = resp.Decode(result); err != nil {
		return nil, err
	}
	return result, nil
}

// ConferenceList возвращает список конференций.
func (c *MXConn) ConferenceList() ([]*Conference, error) {
	resp, err := c.SendWithResponse(&struct {
		XMLName xml.Name `xml:"GetConfList"`
		OwnerID mx.JID   `xml:"ownerId"`
		Iter    int32    `xml:"iter"`
	}{
		OwnerID: c.JID,
	})
	if err != nil {
		return nil, err
	}
	var result = new(struct {
		Conferences []*Conference `xml:"conferences>conf"`
	})
	if err = resp.Decode(result); err != nil {
		return nil, err
	}
	return result.Conferences, nil
}
