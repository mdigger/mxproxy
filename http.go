package main

import (
	"fmt"
	"net"
	"net/http"

	"github.com/mdigger/csta"
	"github.com/mdigger/rest"
)

// Authorize проверяет авторизацию в заголовке HTTP-запроса
func Authorize(c *rest.Context) (login, password string, err error) {
	var ok bool
	login, password, ok = c.BasicAuth()
	if !ok {
		c.SetHeader("WWW-Authenticate", fmt.Sprintf("Basic realm=%s", appName))
		err = rest.ErrUnauthorized
	} else {
		c.AddLogField("login", login) // добавим логин в лог
	}
	return
}

// httpError добавляет к ошибкам код окончания, в зависимости от их типа.
func httpError(c *rest.Context, err error) error {
	// добавляем к ошибкам код окончания для HTTP, чтобы не делать
	// это несколько раз в разных местах.
	if errLogin, ok := err.(*csta.LoginError); ok {
		err = c.Error(http.StatusForbidden, errLogin.Error())
	} else if errNetwork, ok := err.(net.Error); ok && errNetwork.Timeout() {
		err = c.Error(http.StatusGatewayTimeout, errNetwork.Error())
	} else {
		err = c.Error(http.StatusServiceUnavailable, err.Error())
	}
	return err
}
