package main

import (
	"bytes"
	"encoding/gob"
	"time"

	"github.com/boltdb/bolt"
	"github.com/mdigger/csta"
	"github.com/mdigger/log"
	"github.com/mdigger/mx"
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
	log.WithField("file", filename).Info("tokens db")
	return &Store{db: db}, nil
}

// Close закрывает хранилище токенов устройств.
func (s *Store) Close() error {
	log.Info("tokens db closed")
	return s.db.Close()
}

// bucketUsersName задает название раздела с пользователями.
var bucketUsersName = []byte("#users")

// TokenAdd добавляет токен с привязкой к пользователю и приложению.
func (s *Store) TokenAdd(mxuser, topicID, token string) error {
	ctxlog := log.WithFields(log.Fields{
		"user":   mxuser,
		"bundle": topicID,
		"token":  token,
		"type":   "store",
	})
	return s.db.Update(func(tx *bolt.Tx) error {
		// открываем раздел токенов для указанного приложения
		bBundle, err := tx.CreateBucketIfNotExists([]byte(topicID))
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
		var userID = bBundle.Get([]byte(token))
		// если токен был ассоциирован с другим пользователем, то сначала
		// удаляем эту ассоциацию
		if userID != nil && string(userID) != mxuser {
			ctxlog := ctxlog.WithField("user", string(userID))
			// получаем данные о пользователе, удаляем токен и сохраняем
			tokens := ParseTokens(bUsers.Get(userID))
			ctxlog.Debug("remove user token")
			tokens.Remove(topicID, token)
			if len(tokens) == 0 {
				ctxlog.Debug("remove empty user")
				err = bUsers.Delete(userID)
			} else {
				err = bUsers.Put(userID, tokens.Bytes())
			}
			if err != nil {
				return err
			}
		}
		// получаем данные о пользователе, добавляем токен и сохраняем
		var tokens = ParseTokens(bUsers.Get([]byte(mxuser)))
		ctxlog.Debug("add token")
		tokens.Add(topicID, token)
		if err := bUsers.Put([]byte(mxuser), tokens.Bytes()); err != nil {
			return err
		}
		// сохраняем ассоциацию токена и пользователя
		return bBundle.Put([]byte(token), []byte(mxuser))
	})
}

// TokenRemove удаляет токен из хранилища.
func (s *Store) TokenRemove(topicID, token string) error {
	ctxlog := log.WithFields(log.Fields{
		"bundle": topicID,
		"token":  token,
		"type":   "store",
	})
	return s.db.Update(func(tx *bolt.Tx) error {
		// открываем раздел с токенами для приложения
		var bBundle = tx.Bucket([]byte(topicID))
		if bBundle == nil {
			return nil // ни одного токена для приложения нет
		}
		// получаем идентификатор пользователя, ассоциированный с данным токеном
		var userID = bBundle.Get([]byte(token))
		if userID == nil {
			return nil // токен не зарегистрирован
		}
		ctxlog = ctxlog.WithField("user", string(userID))
		// удаляем запись с токеном из хранилища
		ctxlog.Debug("remove bundle token")
		bBundle.Delete([]byte(token))
		// открываем раздел с пользователями
		var bUsers = tx.Bucket(bucketUsersName)
		if bUsers == nil {
			return nil // раздел с пользователями пустой
		}
		// получаем данные о пользователе, удаляем токен и сохраняем
		var tokens = ParseTokens(bUsers.Get(userID))
		ctxlog.Debug("remove user token")
		tokens.Remove(topicID, token)
		var err error
		if len(tokens) == 0 {
			ctxlog.Debug("remove empty user")
			err = bUsers.Delete(userID)
		} else {
			err = bUsers.Put(userID, tokens.Bytes())
		}
		return err
	})
}

// Tokens возвращает список токенов пользователя, сгруппированный по типу
// и идентификатору сертификата.
func (s *Store) Tokens(mxuser string) (Tokens, error) {
	var tokens Tokens
	if err := s.db.View(func(tx *bolt.Tx) error {
		if bUsers := tx.Bucket(bucketUsersName); bUsers != nil {
			tokens = ParseTokens(bUsers.Get([]byte(mxuser)))
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return tokens, nil
}

// Users возвращает список идентификиторов пользователей, сохраненных в
// хранилище, которые относятся к указанному MX серверу.
func (s *Store) Users(mxID string) ([]mx.JID, error) {
	var mxprefix = []byte(mxID + ":")
	var users = make([]mx.JID, 0)
	if err := s.db.View(func(tx *bolt.Tx) error {
		var bUsers = tx.Bucket(bucketUsersName)
		if bUsers == nil {
			return nil
		}
		var cursor = bUsers.Cursor()
		for k, _ := cursor.Seek(mxprefix); k != nil &&
			bytes.HasPrefix(k, mxprefix); k, _ = cursor.Next() {
			jid, err := csta.ParseJID(string(k[len(mxprefix):]))
			if err == nil {
				users = append(users, mx.JID(jid))
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return users, nil
}

// Tokens описывает список токенов, ассоциированных с пользователем.
// Первым ключем идет тип токена (apn|fcm) и topic id, потом сам токен
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

// Bytes возвращает бинарное представление списка токенов. Для кодирования
// используется формат gob.
func (u Tokens) Bytes() []byte {
	var buf = new(bytes.Buffer)
	gob.NewEncoder(buf).Encode(u)
	return buf.Bytes()
}

// Add добавляет в список новый токен устройства с привязкой к идентификатору
// приложения. Если такой токен уже был в списке, то обновляется время
// его создания.
func (u Tokens) Add(topicID, token string) {
	var tokens = u[topicID]
	if tokens == nil {
		tokens = make(map[string]time.Time)
	}
	tokens[token] = time.Now().UTC()
	u[topicID] = tokens
}

// Remove удаляет из списка токен устройства с привязкой к идентификатору
// приложения. Если такой токен раньше не был в списке, ошибки не происходит.
func (u Tokens) Remove(topicID, token string) {
	var tokens = u[topicID]
	delete(tokens, token)
	if len(tokens) == 0 {
		delete(u, topicID)
	}
}
