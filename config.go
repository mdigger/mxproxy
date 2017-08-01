package main

// Config описывает структуру конфигурации сервиса.
type Config struct {
	// список MX серверов и логинов с паролями для серверной авторизации
	// ключ используется в качестве имени MX в URL запроса
	MXList map[string]MXConfig `json:"mx"`
	// список файлов с сертификатами APNS и паролями для их чтения
	APNS map[string]string `json:"apns"`
	// список идентификаторов приложений Android и ассоциированных с ними
	// ключами
	GFCM map[string]string `json:"gfcm"`
}

// MXConfig описывает адрес сервера MX и данные для авторизации.
type MXConfig struct {
	Addr     string `json:"addr"`     // адрес сервера
	Login    string `json:"login"`    // серверный логин
	Password string `json:"password"` // пароль
}
