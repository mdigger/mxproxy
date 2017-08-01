package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"io/ioutil"
	"net/http"
	"time"

	"golang.org/x/crypto/pkcs12"
	"golang.org/x/net/http2"
)

// APNSCert описывает подготовленный и инициализированный транспорт для APNS.
type APNSCert struct {
	BundleID    string       // идентификатор поддерживаемого приложения
	Development bool         // флаг поддержки sandbox
	Production  bool         // флаг поддержки основного приложения
	Expire      time.Time    // дата валидности сертификата
	Client      *http.Client // инициализированный клиент для отправки push
}

// LoadAPNSCertificate загружает сертификат из файла и подготавливает
// HTTP-транспорт для отправки Apple Push Notification.
func LoadAPNSCertificate(filename, password string) (*APNSCert, error) {
	// загружаем содержимое файла с сертификатом
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	// разбираем сертификат
	privateKey, x509Cert, err := pkcs12.Decode(data, password)
	if err != nil {
		return nil, err
	}
	// проверяем валидность сертификата
	if _, err = x509Cert.Verify(x509.VerifyOptions{}); err != nil {
		if _, ok := err.(x509.UnknownAuthorityError); !ok {
			return nil, err
		}
	}
	// инициализируем поддержку HTTP/2
	transport := &http.Transport{
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
	if err := http2.ConfigureTransport(transport); err != nil {
		return nil, err
	}
	// заполняем информацию о сертификате
	apns := &APNSCert{
		Expire: x509Cert.NotAfter,
		Client: &http.Client{
			Transport: transport,
			Timeout:   PushTimeout,
		},
	}
	// добавляем информацию о BundleID
	for _, attr := range x509Cert.Subject.Names {
		if attr.Type.Equal(typeBundle) {
			apns.BundleID = attr.Value.(string)
			break
		}
	}
	// смотрим флаги сертификата о поддержку APNS sandbox
	for _, attr := range x509Cert.Extensions {
		switch t := attr.Id; {
		case t.Equal(typeDevelopmet): // Development
			apns.Development = true
		case t.Equal(typeProduction): // Production
			apns.Production = true
		case t.Equal(typeTopics): // Topics
			// не поддерживаем сертификаты с несколькими темами, т.к. для них
			// нужна более сложная работа
			return nil, ErrAPNSCertificateWithTopics
		}
	}
	return apns, nil
}

var ErrAPNSCertificateWithTopics = errors.New("apns certificate with topics not supported")

var (
	typeBundle     = asn1.ObjectIdentifier{0, 9, 2342, 19200300, 100, 1, 1}
	typeDevelopmet = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 3, 1}
	typeProduction = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 3, 2}
	typeTopics     = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 3, 6}
)
