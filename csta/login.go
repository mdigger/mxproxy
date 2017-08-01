package csta

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"time"
)

// Login описывает параметры для авторизации пользователя, используемые
// сервером MX.
type Login struct {
	UserName           string `xml:"userName" json:"userName"`
	Password           string `xml:"pwd" json:"password"`
	Type               string `xml:"type,attr,omitempty" json:"type,omitempty"`
	ServerType         string `xml:"serverType,attr,omitempty" json:"serverType,omitempty"`
	Platform           string `xml:"platform,attr,omitempty" json:"platform,omitempty"`
	Version            string `xml:"version,attr,omitempty" json:"version,omitempty"`
	LoginCompatibility string `xml:"loginCapab,attr,omitempty" json:"loginCapab,omitempty"`
	MediaCompatibility string `xml:"mediaCapab,attr,omitempty" json:"mediaCapab,omitempty"`
}

// AuthInfo описывает информацию об авторизации на сервере MX.
type AuthInfo struct {
	SN  string `xml:"sn,attr" json:"sn,omitempty"`
	Ext string `xml:"ext,attr" json:"ext,omitempty"`
	JID JID    `xml:"userId,attr" json:"jid,string"`
}

// LoginError описывает ошибку авторизации пользователя.
type LoginError struct {
	Code       uint8  `xml:"Code,attr" json:"code,omitempty"`
	SN         string `xml:"sn,attr" json:"sn,omitempty"`
	APIVersion uint8  `xml:"apiversion,attr" json:"apiVersion,omitempty"`
	Message    string `xml:",chardata" json:"message,omitempty"`
}

// Error возвращает строку с описанием причины ошибки авторизации.
func (e *LoginError) Error() string {
	return e.Message
}

// login выполняет авторизацию пользователя. В случае неверной авторизации
// возвращает ошибку LoginError. После авторизации сохраняет внутренний
// номер и идентификатор пользователя в глобальных свойствах.
func (c *Client) login(login Login) error {
	// хешируем пароль, если он уже не в виде хеша
	var hashed bool             // флаг зашифрованного пароля
	var passwd = login.Password // пароль пользователя для авторизации
	// эвристическим способом проверяем, что пароль похож на base64 от sha1.
	if len(passwd) > 4 && passwd[len(passwd)-1] == '\n' {
		data, err := base64.StdEncoding.DecodeString(passwd[:len(passwd)-1])
		hashed = (err == nil && len(data) == sha1.Size)
	}
	// если пароль еще не представлен в виде base64 от sha1, то делаем это
	if !hashed {
		pwdHash := sha1.Sum([]byte(passwd))
		passwd = base64.StdEncoding.EncodeToString(pwdHash[:]) + "\n"
	}
	// формируем команду для авторизации пользователя
	cmd := &struct {
		XMLName  xml.Name `xml:"loginRequest"`
		Login             // копируем все параметры логина
		Password string   `xml:"pwd"` // заменяем пароль на хеш
	}{Login: login, Password: passwd}
send:
	id, err := c.Send(cmd)
	if err != nil {
		return err
	}
	c.conn.SetReadDeadline(time.Now().Add(ReadTimeout))
read:
	response, err := c.Receive()
	if err != nil {
		return err
	}
	if response.ID != id { // игнорируем все ответы, кроме как на логин
		goto read
	}
	switch response.Name {
	case "loginResponce": // пользователь успешно авторизован
		c.conn.SetReadDeadline(time.Time{})
		return response.Decode(&c.AuthInfo)
	case "loginFailed": // ошибка авторизации
		var loginError = new(LoginError)
		if err := response.Decode(loginError); err != nil {
			return err
		}
		// если ошибка связана с тем, что пароль передан в виде хеш,
		// то повторяем попытку авторизации с паролем в открытом виде
		if hashed && loginError.APIVersion > 2 &&
			(loginError.Code == 2 || loginError.Code == 4) {
			hashed = false
			cmd.Password = login.Password
			goto send // повторяем с открытым паролем
		}
		return loginError // возвращаем ошибку авторизации
	default: // неизвестный ответ, который мы не знаем как разбирать
		return fmt.Errorf("unknown login response %s", response.Name)
	}
}
