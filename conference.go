package main

import "github.com/mdigger/mx"

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
