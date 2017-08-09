package main

import (
	"sync"
	"time"

	"github.com/mdigger/csta"
	"github.com/mdigger/log"
)

// AuthCacheDuration содержит продолжительность, в течение которого
// считается, что пароль пользователя валидный и не требуется повторная проверка.
var AuthCacheDuration = time.Minute * 30

// cacheItem описывает кешируемую информацию об авторизации пользователя.
type cacheItem struct {
	Password string    // пароль пользователя
	csta.JID           // уникальный идентификатор пользователя в рамках MX
	Updated  time.Time // время внесения в кеш
}

// MXAuthCache содержит кеш с информацией об авторизации пользователей на
// сервера MX. Используется чтобы избежать повторной авторизации пользователя
// только для проверки верности логина и пароля.
type MXAuthCache struct {
	list map[string]cacheItem // список пользователей и их паролей
	mu   sync.RWMutex
}

// Check возвращает true, если пользователь с таким логином и паролем
// присутствует в списке и время последней проверки не превышает максимально
// допустимого.
func (a *MXAuthCache) Check(login, password string) (csta.JID, bool) {
	a.mu.RLock()
	p, ok := a.list[login]
	a.mu.RUnlock()
	ok = ok && (time.Since(p.Updated) < AuthCacheDuration)
	log.WithFields(log.Fields{
		"login":   login,
		"inCache": ok,
	}).Debug("check user login cache")
	return p.JID, ok
}

// Add добавляет информацию об авторизации пользователя в кеш.
func (a *MXAuthCache) Add(login, password string, jid csta.JID) {
	a.mu.Lock()
	if a.list == nil {
		a.list = make(map[string]cacheItem)
	}
	if _, ok := a.list[login]; !ok {
		log.WithField("login", login).Debug("add user login to cache")
	}
	a.list[login] = cacheItem{
		JID:      jid,
		Password: password,
		Updated:  time.Now(),
	}
	a.mu.Unlock()
}
