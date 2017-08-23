package main

import (
	"github.com/mdigger/log"
	"github.com/mdigger/mx"
)

// MXClient описывает клиентское подключение к MX.
type MXClient struct {
	conn *mx.Conn     // активное пользовательское подключение
	log  *log.Context // для вывода лога
}

// ConnectMXClient устанавливает клиентское соединение с сервером MX и возвращает его.
func ConnectMXClient(host, login, password string) (*MXClient, error) {
	conn, err := mx.Connect(host, mx.Login{
		UserName: login,
		Password: password,
		Type:     "User",
		Platform: "iPhone",
		Version:  "1.0",
	})
	if err != nil {
		return nil, err
	}
	var client = &MXClient{
		conn: conn,
		log: log.WithFields(log.Fields{
			"mx":    conn.SN,
			"type":  "user",
			"login": login,
		}),
	}
	client.log.Debug("connected to mx")
	return client, nil
}

// Close закрывает соединение с сервером MX.
func (c *MXClient) Close() error {
	// останавливаем пользовательский монитор, если он был запущен
	c.conn.MonitorStopID(0)
	c.log.Debug("disconnected from mx")
	return c.conn.Close()
}
