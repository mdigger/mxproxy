package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"io/ioutil"
	"net/http"

	"github.com/mdigger/log"
	"golang.org/x/crypto/pkcs12"
	"golang.org/x/net/http2"
)

// APNS содержит список инициализированных транспортов для отправки Apple
// Push Notification ассоциированный с идентификатором темы (topic ID).
type APNS struct {
	transports map[string]*http.Transport
}

// Support возвращает true, если указанная тема поддерживается сертификатами.
func (apns *APNS) Support(topicID string) bool {
	_, ok := apns.transports[topicID]
	return ok
}

// Get возвращает инициализированный http транспорт для указанной тему. Если
// указанная тема не поддерживается, то возвращается nil.
func (apns *APNS) Get(topicID string) *http.Transport {
	return apns.transports[topicID]
}

// LoadCertificate загружает сертификат для Apple Push и сохраняеи во внутреннем
// списке подготовленный для него http.Transport.
func (apns *APNS) LoadCertificate(filename, password string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	privateKey, x509Cert, err := pkcs12.Decode(data, password)
	if err != nil {
		return err
	}
	if _, err = x509Cert.Verify(x509.VerifyOptions{}); err != nil {
		if _, ok := err.(x509.UnknownAuthorityError); !ok {
			return err
		}
	}
	var topicID string
	for _, attr := range x509Cert.Subject.Names {
		if attr.Type.Equal(typeBundle) {
			topicID = attr.Value.(string)
			break
		}
	}
	var transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{
				tls.Certificate{
					Certificate: [][]byte{x509Cert.Raw},
					PrivateKey:  privateKey,
					Leaf:        nil,
				},
			},
		},
	}
	if err = http2.ConfigureTransport(transport); err != nil {
		return err
	}
	if apns.transports == nil {
		apns.transports = make(map[string]*http.Transport)
	}
	for _, attr := range x509Cert.Extensions {
		switch t := attr.Id; {
		case t.Equal(typeDevelopmet): // Development
			apns.transports[topicID+"~"] = transport
		case t.Equal(typeProduction): // Production
			apns.transports[topicID] = transport
		case t.Equal(typeTopics): // Topics
			// не поддерживаем сертификаты с несколькими темами, т.к. для них
			// нужна более сложная работа
			return errors.New("apns certificate with topics not supported")
		}
	}
	log.WithFields(log.Fields{
		"file":    filename,
		"topic":   topicID,
		"expires": x509Cert.NotAfter.Format("2006-01-02"),
	}).Info("apns certificate")
	return nil
}

var (
	typeBundle     = asn1.ObjectIdentifier{0, 9, 2342, 19200300, 100, 1, 1}
	typeDevelopmet = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 3, 1}
	typeProduction = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 3, 2}
	typeTopics     = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 3, 6}
)
