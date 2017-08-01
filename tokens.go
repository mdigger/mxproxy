package main

import (
	"bytes"
	"encoding/gob"
	"time"
)

// Tokens описывает список токенов, ассоциированных с пользователем.
// Первым ключем идет тип токена (apns|gfcm) и bundle id, потом сам токен
// устройства. В качестве значения задается время последнего внесения токена
// в список.
type Tokens map[string]map[string]time.Time

// ParseTokens восстанавливает список токенов пользователя из их
// бинарного представления в формате gob.
func ParseTokens(data []byte) Tokens {
	tokens := make(Tokens)
	gob.NewDecoder(bytes.NewReader(data)).Decode(&tokens)
	return tokens
}

// Encode возвращает бинарное представление списка токенов. Для кодирования
// используется формат gob.
func (u Tokens) Bytes() []byte {
	var buf = new(bytes.Buffer)
	gob.NewEncoder(buf).Encode(u)
	return buf.Bytes()
}

// Add добавляет в список новый токен устройства с привязкой к идентификатору
// приложения. Если такой токен уже был в списке, то обновляется время
// его создания.
func (u Tokens) Add(bundle, token string) {
	var tokens = u[bundle]
	if tokens == nil {
		tokens = make(map[string]time.Time)
	}
	tokens[token] = time.Now().UTC()
	u[bundle] = tokens
}

// Remove удаляет из списка токен устройства с привязкой к идентификатору
// приложения. Если такой токен раньше не был в списке, ошибки не происходит.
func (u Tokens) Remove(bundle, token string) {
	var tokens = u[bundle]
	delete(tokens, token)
	if len(tokens) == 0 {
		delete(u, bundle)
	}
}
