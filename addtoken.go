package main

import (
	"fmt"
	"net/http"

	"github.com/mdigger/rest"
)

// AddToken добавляет токен устройства в хранилище.
func (mx *MX) AddToken(c *rest.Context) error {
	// проверяем и формируем идентификатор токена
	var (
		bundleID  = c.Param("bundle")
		tokenType = c.Param("type")
	)
	switch tokenType {
	case "apns":
		// проверяем, что взведен флаг sandbox
		if _, sandbox := c.Request.URL.Query()["sandbox"]; sandbox {
			bundleID += "~"
		}
		// проверяем, что сертификат с таким bundleID зарегистрирован
		if _, ok := apnsClients[bundleID]; !ok {
			return c.Error(http.StatusNotFound, "unsupported apns bundle id")
		}
	case "gfcm":
		if _, ok := gfcmKeys[bundleID]; !ok {
			return c.Error(http.StatusNotFound, "unsupported google bundle id")
		}
	default:
		return c.Error(http.StatusNotFound, "bad push type")
	}
	// объединяем bundleID c типом
	bundleID = fmt.Sprintf("%s:%s", tokenType, bundleID)
	// проверяем авторизацию пользователя
	login, password, err := Authorize(c)
	if err != nil {
		return err
	}
	// получаем идентификатор пользователя MX
	jid, ok := mx.authCache.Check(login, password)
	if !ok {
		// если в кеше нет записи, то авторизуем пользователя
		client, err := mx.UserClient(login, password)
		if err != nil {
			return httpError(c, err)
		}
		jid = client.JID
		client.Close() // закрываем клиента в случае успешной авторизации
	}
	var userID = fmt.Sprintf("%s:%s", mx.SN, jid)
	// разбираем данные запроса
	var data = new(struct {
		Token string `json:"token" form:"token"`
	})
	if err := c.Bind(data); err != nil {
		return err
	}
	if len(data.Token) < 20 {
		return c.Error(http.StatusBadRequest, "bad push token")
	}
	mx.CallMonsStart(jid) // запускаем мониторинг пользователя
	// сохраняем токен в хранилище
	return storeDB.Add(userID, bundleID, data.Token)
}
