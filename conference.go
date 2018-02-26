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
