package csta

import "encoding/xml"

// Response описывает формат входящего от сервера MX сообщения.
type Response struct {
	ID   uint16 // идентификатор сообщения
	Name string // название команды
	data []byte // неразобранная команда в формате XML
}

// Decode декодирует сообщение в указанный объект.
func (m *Response) Decode(v interface{}) error {
	return xml.Unmarshal(m.data, v)
}
