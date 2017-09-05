package main

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/mdigger/rest"
)

var (
	httpClient = http.Client{Timeout: time.Second * 10}
)

// GetProvisioning запрашивает и разбирает конфигурацию пользователя с
// сервера провижининга.
func (p *Proxy) GetProvisioning(login, password string) (*MXConfig, error) {
	req, err := http.NewRequest("GET", p.provisioningURL, nil)
	if err != nil {
		return nil, rest.NewError(http.StatusInternalServerError, err.Error())
	}
	req.SetBasicAuth(login, password)   // добавляем авторизацию
	req.Header.Set("User-Agent", agent) // добавляем имя агента для запроса
	resp, err := httpClient.Do(req)     // делаем запрос на получение конфигурации
	if err != nil {
		var status = http.StatusServiceUnavailable
		if errTimeout, ok := err.(net.Error); ok && errTimeout.Timeout() {
			status = http.StatusGatewayTimeout
		}
		return nil, rest.NewError(status, err.Error())
	}
	defer resp.Body.Close()
	// проверяем, что ответ не содержит ошибки
	if resp.StatusCode != http.StatusOK {
		return nil, rest.NewError(resp.StatusCode,
			http.StatusText(resp.StatusCode))
	}
	// разбираем полученную конфигурацию
	var config = new(struct {
		MX *struct {
			Login    string `json:"account_name"`
			Password string `json:"account_pwd"`
			MXHost   string `json:"address"`
			Port     string `json:"csta_port"`
			SSL      bool   `json:"csta_ssl"`
			MXID     string `json:"sn"`
		} `json:"MX"`
	})
	if err = json.NewDecoder(resp.Body).Decode(config); err != nil {
		return nil, rest.NewError(http.StatusBadGateway, err.Error())
	}
	// проверяем, что все необходимые данные присутствуют
	if config.MX.Login == "" ||
		config.MX.Password == "" ||
		config.MX.MXHost == "" ||
		config.MX.Port == "" {
		return nil, rest.NewError(http.StatusForbidden,
			"mx provisioning is not configured")
	}
	if !config.MX.SSL {
		return nil, rest.NewError(http.StatusForbidden,
			"unprotected connection to mx server is not supported")
	}
	return &MXConfig{
		Host:     net.JoinHostPort(config.MX.MXHost, config.MX.Port),
		Login:    config.MX.Login,
		Password: config.MX.Password,
	}, nil
}

// MXConfig описывает конфигурацию пользователя, получаемую с сервера
// провижининга.
type MXConfig struct {
	Host     string `json:"host"`             // адрес сервера MX, включая порт
	Login    string `json:"login" jwt:"sub"`  // логин для авторизации
	Password string `json:"password" jwt:"-"` // пароль пользователя
}
