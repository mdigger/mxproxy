package main

import "time"

// Duration переопределяет для стандартной time.Duration представление в
// JSON формате в виде строки.
type Duration struct {
	time.Duration
}

// UnmarshalJSON восстанавливает временной интервал из строки
func (d *Duration) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var err error
	d.Duration, err = time.ParseDuration(string(data[1 : len(data)-1]))
	return err
}
