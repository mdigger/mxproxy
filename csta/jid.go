package csta

import "strconv"

// JID описывает формат уникального идентификатора, используемого сервером MX.
type JID uint64

// ParseJID разбирает строковое представление JID.
func ParseJID(jid string) (JID, error) {
	persedJID, err := strconv.ParseUint(jid, 10, 64)
	return JID(persedJID), err
}

// String возвращает строковое представление идентификатора.
func (jid JID) String() string {
	return strconv.FormatUint(uint64(jid), 10)
}

// MarshalJSON отдает представление уникального идентификатора в формате JSON.
func (jid JID) MarshalJSON() ([]byte, error) {
	return []byte(jid.String()), nil
}

// UnmarshalJSON восстанавливает уникальный идентификатор из формата JSON.
func (jid *JID) UnmarshalJSON(data []byte) error {
	id, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return err
	}
	*jid = JID(id)
	return nil
}
