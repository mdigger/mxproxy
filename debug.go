package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/kr/pretty"
)

// sendMonitorText отправляет данные на сервер мониторинга.
func sendMonitorText(text string) error {
	if sendMonitor == nil {
		return nil
	}
	if sendFooter == "" {
		sendFooter = fmt.Sprintf("\n────────────────\n"+
			"Service: %s\nVersion: %s\nBuilded: %s\nGit: %s\nHost: %s",
			appName, version, date, git, host)
	}
	return sendMonitor(text + sendFooter)
}

// sendMonitorError отсылает ошибку.
func sendMonitorError(err error) error {
	var msg = fmt.Sprintf("Error: *%s*\n`%# v`\n",
		err.Error(), pretty.Formatter(err))
	var pc = make([]uintptr, 10)
	if n := runtime.Callers(2, pc); n > 0 {
		pc = pc[:n]
		var frames = runtime.CallersFrames(pc)
		var stack = make([]string, 0, n)
		for {
			frame, more := frames.Next()
			if strings.Contains(frame.File, "runtime/") {
				break
			}
			stack = append(stack, fmt.Sprintf("- `%s` (%s#L%d)",
				frame.Function, frame.File, frame.Line))
			if !more {
				break
			}
		}
		msg += strings.Join(stack, "\n")
	}
	return sendMonitorText(msg)
}

var (
	sendMonitor func(string) error // реальная функция отправки
	sendFooter  string             // подвал сообщения
)
