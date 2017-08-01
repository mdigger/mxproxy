package csta

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
)

var (
	// флаг вывода в консоль в цвете
	// устанавливается автоматически при задании SetLogOutput, если
	// поддерживается ASCII
	LogTTY = false
	// символы, используемые для вывода направления (true - входящие,
	// false - исходящие)
	LogINOUT = map[bool]string{true: "→", false: "←"}
	// строка с форматированием вывода в лог
	// передаются следующие данные: направление [1], номер команды [2], сама
	// команда в формате XML [3]
	LogFormat = "%s %s %s"
	// лог, используемый для вывода команд CSTA
	cstaLogger = log.New(ioutil.Discard, "", log.LstdFlags)
	output     = false
)

// SetLogOutput задает куда выводить лог с командами CSTA.
func SetLogOutput(w io.Writer) {
	cstaLogger.SetOutput(w)
	output = (w != ioutil.Discard)
	if out, ok := w.(*os.File); ok {
		if fi, err := out.Stat(); err == nil {
			LogTTY = fi.Mode()&(os.ModeDevice|os.ModeCharDevice) != 0
		}
	}
}

// SetLogFlags задает флаги для вывода лога CSTA.
func SetLogFlags(flag int) {
	cstaLogger.SetFlags(flag)
}

// log форматируем вывод лога с командами CSTA.
func (c *Client) log(inFlag bool, id uint16, data []byte) {
	if !output {
		return
	}
	var fmtID = "%04d"
	if LogTTY {
		// добавляем цветовое выделение к идентификатору команды или ответа
		switch id {
		case 0:
			fmtID = "\033[37m" + "%04d" + "\033[0m"
		case 9999:
			fmtID = "\033[34m" + "%04d" + "\033[0m"
		default:
			fmtID = "\033[33m" + "%04d" + "\033[0m"
		}
		// выделяем цветом название команды или ответа
		indx := bytes.IndexAny(data, ">/ ")
		data = []byte(fmt.Sprintf("<\033[32m%s\033[0m%s",
			data[1:indx], data[indx:]))
	}
	cstaLogger.Printf(LogFormat, LogINOUT[inFlag], fmt.Sprintf(fmtID, id), data)
}
