package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mdigger/log"
	"github.com/mdigger/rest"
	bolt "go.etcd.io/bbolt"
)

// Store описывает хранилище данных
type Store struct {
	db *bolt.DB
}

// OpenStore открывает и возвращает хранилище данных.
func OpenStore(filename string) (*Store, error) {
	db, err := bolt.Open(filename, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	log.Info("db opened", "file", filename)
	return &Store{db: db}, nil

}

// Close закрывает хранилище данных.
func (s *Store) Close() error {
	log.Info("db closed")
	return s.db.Close()
}

// Названия разделов в хранилище
const (
	bucketUsers  = "users"
	bucketTokens = "tokens"
	// bucketApps   = "apps"
)

// AddUser добавляет информацию о пользователе в хранилище.
func (s *Store) AddUser(login string, config *MXConfig) error {
	return s.add(bucketUsers, login, config)
}

// RemoveUser удаляет информацию о пользователе из хранилища.
func (s *Store) RemoveUser(login string) error {
	return s.remove(bucketUsers, login)
}

// GetUser возвращает информацию о пользователе из хранилища.
func (s *Store) GetUser(login string) (*MXConfig, error) {
	var conf = new(MXConfig)
	if err := s.get(bucketUsers, login, conf); err != nil {
		return nil, err
	}
	return conf, nil
}

// ListUsers возвращает список зарегистрированных пользователей.
func (s *Store) ListUsers() []string {
	return s.list(bucketUsers)
}

// AddToken добавляет токен устройства в хранилище.
func (s *Store) AddToken(kind, topic, token, login string) error {
	return s.add(bucketTokens, kind+":"+topic+":"+token, login)
}

// RemoveToken удаляет токен из хранилища.
func (s *Store) RemoveToken(kind, topic, token string) error {
	return s.remove(bucketTokens, kind+":"+topic+":"+token)
}

// ListTokens возвращает список токенов пользователя указанного типа.
func (s *Store) ListTokens(kind, topic, login string) []string {
	var list []string
	s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketTokens))
		if bucket == nil {
			return nil
		}
		var (
			key    = []byte(kind + ":" + topic + ":")
			cursor = bucket.Cursor()
		)
		for k, v := cursor.Seek(key); k != nil && bytes.HasPrefix(k, key); k, v = cursor.Next() {
			if bytes.Equal(v, []byte(login)) {
				list = append(list, string(k[len(key):]))
			}
		}
		return nil
	})
	return list
}

// add сохраняет объект в указанном разделе хранилище с заданным ключом.
func (s *Store) add(section, key string, obj interface{}) error {
	var data []byte
	switch obj := obj.(type) {
	case []byte:
		data = obj
	case string:
		data = []byte(obj)
	case fmt.Stringer:
		data = []byte(obj.String())
	default:
		var err error
		if data, err = json.Marshal(obj); err != nil {
			return err
		}
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(section))
		if err != nil {
			return err
		}
		return bucket.Put([]byte(key), data)
	})
}

// ErrNotFound возвращается, если в хранилище нет данных с таким ключом.
var ErrNotFound = rest.ErrNotFound

// remove удаляет данные с заданным ключом из указанного раздела хранилища. Если
// данных с таким ключом в хранилище не найдено, то возвращает ошибку
// ErrNotFound.
func (s *Store) remove(section, key string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if bucket := tx.Bucket([]byte(section)); bucket != nil {
			if bucket.Get([]byte(key)) == nil {
				return ErrNotFound
			}
			return bucket.Delete([]byte(key))
		}
		return nil
	})
}

// get возвращает данные с заданным ключом из указанного раздела хранилище.
func (s *Store) get(section, key string, obj interface{}) error {
	var data []byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		if bucket := tx.Bucket([]byte(section)); bucket != nil {
			if data = bucket.Get([]byte(key)); data == nil {
				return ErrNotFound
			}
		}
		return nil
	}); err != nil {
		return err
	}
	switch obj := obj.(type) {
	case []byte:
		obj = data
	case *string:
		*obj = string(data)
	default:
		return json.Unmarshal(data, obj)
	}
	return nil
}

// list возвращает список ключей в указанном разделе хранилища.
func (s *Store) list(section string) []string {
	var list []string
	s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(section))
		if bucket == nil {
			return nil
		}
		list = make([]string, 0, bucket.Stats().KeyN)
		return bucket.ForEach(func(k, _ []byte) error {
			list = append(list, string(k))
			return nil
		})
	})
	return list
}

// section возвращает все ключи и их значения в заданном разделе хранилища
func (s *Store) section(section string) map[string]interface{} {
	var result map[string]interface{}
	s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(section))
		if bucket == nil {
			return nil
		}
		result = make(map[string]interface{}, bucket.Stats().KeyN)
		return bucket.ForEach(func(k, v []byte) error {
			if v == nil {
				result[string(k)] = nil
			} else if v[0] == '{' {
				result[string(k)] = json.RawMessage(v)
			} else {
				result[string(k)] = string(v)
			}
			return nil
		})
	})
	return result
}

// keys возвращает количество зарегистрированных в данном резделе данных.
func (s *Store) keys(section string) int {
	var count int
	s.db.View(func(tx *bolt.Tx) error {
		if bucket := tx.Bucket([]byte(section)); bucket != nil {
			count = bucket.Stats().KeyN
		}
		return nil
	})
	return count
}
