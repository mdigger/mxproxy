package main

import (
	"sync"
	"time"

	"github.com/mdigger/mx"
)

// AuthCacheDuration задает время актуальности хранимых в кеше авторизации
// данных.
var AuthCacheDuration = time.Minute * 30

// AuthCache содержит кеш информации об авторизации пользователя.
type AuthCache struct {
	users sync.Map // информация о паролях пользователя и времени логина
}

// Check возвращает уникальный идентификатор пользователя MX если пользователь
// уже авторизовался не так давно с этими данными. В противном случае вернет 0.
func (c *AuthCache) Check(login, password string) mx.JID {
	if data, ok := c.users.Load(login); ok {
		if user := data.(*userCacheItem); user.Password == password &&
			time.Since(user.Updated) < AuthCacheDuration {
			// log.WithField("login", login).Debug("user in cache")
			return user.JID
		}
	}
	return 0
}

// Add добавляет информацию об авторизации пользователя в кеш. Если пользователь
// уже был сохранен в кеше, то обновляется время его последней проверки
// авторизации.
func (c *AuthCache) Add(login, password string, jid mx.JID) {
	c.users.Store(login, &userCacheItem{
		Password: password,
		JID:      jid,
		Updated:  time.Now(),
	})
	// log.WithField("login", login).Debug("add user to cache")
}

// ClearExpired удаляет из кеша все данные об авторизации пользователей,
// которые на данный момент устарели.
func (c *AuthCache) ClearExpired() {
	var checkpoint = time.Now().Add(-AuthCacheDuration)
	c.users.Range(func(login, user interface{}) bool {
		if user := user.(*userCacheItem); user.Updated.Before(checkpoint) {
			c.users.Delete(login)
		}
		return true
	})
}

// userCacheItem описывает хранящиеся в кеше данные для проверки авторизации
// пользователя.
type userCacheItem struct {
	Password string    // пароль пользователя
	mx.JID             // уникальный идентификатор пользователя в рамках MX
	Updated  time.Time // время внесения в кеш
}
