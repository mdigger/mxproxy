package main

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/mdigger/jwt"
	"github.com/mdigger/log"
)

// JWTGenerator описывает данные для генерации и проверки авторизационных токенов.
type JWTGenerator struct {
	id      string        // идентификатор токена
	key     interface{}   // ключ для подписи и проверки токенов авторизации
	created time.Time     // время создания ключа
	mu      sync.RWMutex  // блокировка доступа к ключу
	ttl     time.Duration // время жизни ключа
	old     sync.Map      // архив ключей
	conf    *jwt.Config   // конфигурация для создания токена авторизации
	remover *time.Timer   // удаление старых ключей
}

// NewJWTGenerator инициализирует генератор авторизационных токенов. signKeyTTL
// задает время жизни ключа для подписи токенов, после чего он автоматически
// меняется. А tokenTTL - время жизни токена авторизации.
func NewJWTGenerator(tokenTTL, signKeyTTL time.Duration) *JWTGenerator {
	var jwtConfig = &JWTGenerator{
		conf: &jwt.Config{
			Issuer:   "https://" + host,
			Created:  true,
			Expires:  tokenTTL,
			UniqueID: jwt.Nonce(8),
		},
		ttl: signKeyTTL,
	}
	// задаем функцию для отдачи текущего ключа для подписи токенов
	jwtConfig.conf.Key = jwtConfig.getCurrentKey
	// запускаем удаление старых ключей
	jwtConfig.remover = time.AfterFunc(signKeyTTL, func() {
		var now = strconv.FormatInt(
			time.Now().Add(-signKeyTTL-tokenTTL*2).Unix(), 36)
		jwtConfig.old.Range(func(k, _ interface{}) bool {
			if k.(string) < now {
				jwtConfig.old.Delete(k)
				log.Debug("removed old token sign key", "id", k.(string))
			}
			return true
		})
		jwtConfig.remover.Reset(signKeyTTL)
	})
	return jwtConfig
}

// Close останавливает удаление старых ключей.
func (j *JWTGenerator) Close() {
	j.remover.Stop()
}

// Token возвращает авторизационный токен и описание к нему.
func (j *JWTGenerator) Token(login string) (*TokenDescription, error) {
	token, err := j.conf.Token(login)
	if err != nil {
		return nil, err
	}
	return &TokenDescription{
		Type:    "Bearer",
		Token:   token,
		Expired: j.conf.Expires.Seconds(),
	}, nil
}

// TokenDescription задает формат описания токена для авторизации.
type TokenDescription struct {
	Type    string  `json:"token_type,omitempty"`
	Token   string  `json:"access_token"`
	Expired float64 `json:"expires_in,omitempty"`
}

// getCurrentKey возвращает название и текущий ключ для подписи авторизационных
// токенов.
func (j *JWTGenerator) getCurrentKey() (string, interface{}) {
	j.mu.RLock()
	var id, key, expired = j.id, j.key, time.Since(j.created) > j.ttl
	j.mu.RUnlock()
	// обновляем ключ по необходимости
	if expired {
		var created = time.Now()
		key = jwt.NewES256Key()
		id = strconv.FormatInt(created.Unix(), 36)
		j.mu.Lock()
		j.created = created
		j.key = key
		j.id = id
		j.mu.Unlock()
		j.old.Store(id, key) // сохраняем в архиве
		log.Debug("generated new token sign key", "id", id)
	}
	return id, key
}

// getKey возвращает ключ по его идентификатору.
func (j *JWTGenerator) getKey(alg, id string) interface{} {
	if alg == "ES256" {
		if key, ok := j.old.Load(id); ok {
			return key
		}
	}
	return nil
}

// ErrUnknownSignKey возвращается при верификации токена с устаревшим ключом.
var ErrUnknownSignKey = errors.New("unknown or obsolete signing key")

// Verify проверяет валидность токена и возвращает информацию о логине
// пользователя из него.
func (j *JWTGenerator) Verify(token string) (string, error) {
	if err := jwt.Verify(token, j.getKey); err != nil {
		if err == jwt.ErrEmptySignKey {
			return "", ErrUnknownSignKey
		}
		return "", err
	}
	var t = new(struct {
		Login string `json:"sub"`
	})
	if err := jwt.Decode(token, t); err != nil {
		return "", err
	}
	return t.Login, nil
}
