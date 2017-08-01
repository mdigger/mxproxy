package main

import (
	"bytes"
	"time"

	"github.com/boltdb/bolt"
	"github.com/mdigger/csta"
	"github.com/mdigger/log"
	"github.com/mdigger/rest"
)

// Store предоставляет доступ к хранилищу токенов устройств пользователей для
// пуш-уведомлений, с привязкой к серверам MX и идентификаторам приложений.
type Store struct {
	db *bolt.DB
}

// OpenStore открывает и инициализирует хранилище токенов устройств.
func OpenStore(filename string) (*Store, error) {
	db, err := bolt.Open(filename, 0666, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close закрывает хранилище токенов устройств.
func (s *Store) Close() error {
	return s.db.Close()
}

// bucketUsersName задает название раздела с пользователями.
var bucketUsersName = []byte("#users")

// Add добавляет в хранилище ассоциацию пользователя и токена.
func (s *Store) Add(user, bundle, token string) error {
	ctxlog := log.WithFields(log.Fields{
		"user":   user,
		"bundle": bundle,
		"token":  token,
		"type":   "store",
	})
	return s.db.Update(func(tx *bolt.Tx) error {
		// открываем раздел токенов для указанного приложения
		bBundle, err := tx.CreateBucketIfNotExists([]byte(bundle))
		if err != nil {
			return err
		}
		// открываем раздел с пользователями
		bUsers, err := tx.CreateBucketIfNotExists(bucketUsersName)
		if err != nil {
			return err
		}
		// получаем идентификатор пользователя, ассоциированный с данным
		// токеном, если он был раньше ассоциирован
		userId := bBundle.Get([]byte(token))
		// если токен был ассоциирован с другим пользователем, то сначала
		// удаляем эту ассоциацию
		if userId != nil && string(userId) != user {
			ctxlog := ctxlog.WithField("user", string(userId))
			// получаем данные о пользователе, удаляем токен и сохраняем
			tokens := ParseTokens(bUsers.Get(userId))
			ctxlog.Debug("remove user token")
			tokens.Remove(bundle, token)
			if len(tokens) == 0 {
				ctxlog.Debug("remove empty user")
				err = bUsers.Delete(userId)
			} else {
				err = bUsers.Put(userId, tokens.Bytes())
			}
			if err != nil {
				return err
			}
		}
		// получаем данные о пользователе, добавляем токен и сохраняем
		tokens := ParseTokens(bUsers.Get([]byte(user)))
		ctxlog.Debug("add token")
		tokens.Add(bundle, token)
		if err := bUsers.Put([]byte(user), tokens.Bytes()); err != nil {
			return err
		}
		// сохраняем ассоциацию токена и пользователя
		return bBundle.Put([]byte(token), []byte(user))
	})
}

// Remove удаляет токен из хранилища.
func (s *Store) Remove(bundle, token string) error {
	ctxlog := log.WithFields(log.Fields{
		"bundle": bundle,
		"token":  token,
		"type":   "store",
	})
	return s.db.Update(func(tx *bolt.Tx) error {
		// открываем раздел с токенами для приложения
		bBundle := tx.Bucket([]byte(bundle))
		if bBundle == nil {
			return nil // ни одного токена для приложения нет
		}
		// получаем идентификатор пользователя, ассоциированный с данным токеном
		userId := bBundle.Get([]byte(token))
		if userId == nil {
			return nil // токен не зарегистрирован
		}
		ctxlog = ctxlog.WithField("user", string(userId))
		// удаляем запись с токеном из хранилища
		ctxlog.Debug("remove bundle token")
		bBundle.Delete([]byte(token))
		// открываем раздел с пользователями
		bUsers := tx.Bucket(bucketUsersName)
		if bUsers == nil {
			return nil // раздел с пользователями пустой
		}
		// получаем данные о пользователе, удаляем токен и сохраняем
		tokens := ParseTokens(bUsers.Get(userId))
		ctxlog.Debug("remove user token")
		tokens.Remove(bundle, token)
		var err error
		if len(tokens) == 0 {
			ctxlog.Debug("remove empty user")
			err = bUsers.Delete(userId)
		} else {
			err = bUsers.Put(userId, tokens.Bytes())
		}
		return err
	})
}

// Get возвращает список токенов пользователя.
func (s *Store) Get(user string) (Tokens, error) {
	var tokens Tokens
	if err := s.db.View(func(tx *bolt.Tx) error {
		if bUsers := tx.Bucket(bucketUsersName); bUsers != nil {
			tokens = ParseTokens(bUsers.Get([]byte(user)))
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return tokens, nil
}

// Users возвращает список идентификаторов пользователей, зарегистрированных
// для данного MX.
func (s *Store) Users(mx string) ([]csta.JID, error) {
	var mxprefix = []byte(mx + ":")
	var users = make([]csta.JID, 0)
	if err := s.db.View(func(tx *bolt.Tx) error {
		bUsers := tx.Bucket(bucketUsersName)
		if bUsers == nil {
			return nil
		}
		cursor := bUsers.Cursor()
		for k, _ := cursor.Seek(mxprefix); k != nil &&
			bytes.HasPrefix(k, mxprefix); k, _ = cursor.Next() {
			jid, err := csta.ParseJID(string(k[len(mxprefix):]))
			if err == nil {
				users = append(users, csta.JID(jid))
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return users, nil
}

// json возвращает представление хранилища в виде объекта в формате JSON.
// Используется исключительно в отладочных целях, т.к. при большом объеме
// хранилища может оказаться слишком затратным по памяти.
func (s *Store) json() (rest.JSON, error) {
	var backup = make(rest.JSON)
	if err := s.db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			var section = make(rest.JSON, b.Stats().KeyN)
			if err := b.ForEach(func(k, v []byte) error {
				if bytes.Equal(bucketUsersName, name) {
					section[string(k)] = ParseTokens(v)
				} else {
					section[string(k)] = string(v)
				}
				return nil
			}); err != nil {
				return err
			}
			backup[string(name)] = section
			return nil
		})

	}); err != nil {
		return nil, err
	}
	return backup, nil
}
