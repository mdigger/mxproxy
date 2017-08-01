package csta

import (
	"strconv"
	"time"
)

// Timestamp поддерживает представление временной метки в виде числа секунд с
// момента начала эпохи. Именно это представление времени используется при
// получении событий от сервера MX.
type Timestamp struct {
	time.Time
}

// ParseTimestamp разбирает временную метку из строки.
func ParseTimestamp(timestamp string) Timestamp {
	if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
		return Timestamp{t}
	}
	return Timestamp{}
}

// String возвращает строковое представление времени.
func (t Timestamp) String() string {
	return t.Time.String()
}

func (t Timestamp) Unix() uint64 {
	return uint64(t.Time.Unix())
}

// MarshalText представляет временную метку в виде строки с числом.
func (t Timestamp) MarshalText() ([]byte, error) {
	return []byte(strconv.FormatInt(t.Time.Unix(), 10)), nil
}

// UnmarshalText восстанавливает временную метку из строки с числом.
func (t *Timestamp) UnmarshalText(data []byte) error {
	v, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}
	t.Time = time.Unix(v, 0)
	return nil
}

// MarshalJSON отдает представление временной метки в формате JSON. Используется
// UTC представление времени.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return t.Time.UTC().MarshalJSON()
}

// UnmarshalJSON восстанавливает временную метку из формата JSON.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	return t.Time.UnmarshalJSON(data)
}
