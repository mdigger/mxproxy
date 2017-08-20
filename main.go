package main

import (
	"crypto/tls"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/mdigger/log"
	"github.com/mdigger/mx"
	"github.com/mdigger/rest"
	"golang.org/x/crypto/acme/autocert"
)

// информация о сервисе и версия
var (
	appName = "mxproxy" // название сервиса
	version = "0.15"    // версия
	date    = ""        // дата сборки
	git     = ""        // версия git

	host         = appName + ".connector73.net" // имя сервера
	configName   = appName + ".json"            // имя конфигурационного файла
	tokensDBName = appName + ".db"              // имя файла с хранилищем токенов
)

func init() {
	// инициализируем разбор параметров запуска сервиса
	flag.StringVar(&host, "host", host, "main server `name`")
	flag.StringVar(&configName, "config", configName, "config `filename`")
	flag.StringVar(&tokensDBName, "db", tokensDBName, "tokens DB `filename`")
	var debug = false // флаг вывода отладочной информации
	flag.BoolVar(&debug, "debug", debug, "debug output")
	var cstaOutput bool // флаг вывода команд и ответов CSTA
	flag.BoolVar(&cstaOutput, "csta", cstaOutput, "CSTA output")
	var logFlags = log.Lindent //| mx.Lcolor
	flag.IntVar(&logFlags, "logflag", logFlags, "log flags")
	flag.Parse()

	log.SetFlags(logFlags) // устанваливаем флаги вывода в лог
	// разрешаем вывод отладочной информации, включая вывод команд CSTA
	if debug {
		log.SetLevel(log.DebugLevel)
	}
	if cstaOutput {
		mx.SetCSTALog(os.Stdout, logFlags)
	}
	// выводим информацию о текущей версии
	var verInfoFields = log.Fields{
		"name":    appName,
		"version": version,
	}
	if date != "" {
		verInfoFields["builded"] = date
	}
	if git != "" {
		verInfoFields["git"] = git
	}
	log.WithFields(verInfoFields).Info("service info")
}

func main() {
	// загружаем конфигурацию и устаналвиваем серверные соединения с MX
	proxy, err := LoadConfig(configName, tokensDBName)
	if err != nil {
		log.WithError(err).Error("service initialization error")
		os.Exit(2)
	}
	defer proxy.Close()

	// инициализируем обработку HTTP запросов
	var mux = &rest.ServeMux{
		Headers: map[string]string{
			"Server":            "MXProxy/1.1", // ¯\_(ツ)_/¯
			"X-API-Version":     "1.0",
			"X-Service-Version": version,
		},
		Logger: log.WithField("type", "http"),
	}
	if git != "" {
		mux.Headers["Server"] += " (" + git + ")"
	}
	mux.Handle("GET", "/mx/:mx/contacts", proxy.GetAddressBook)
	mux.Handle("GET", "/mx/:mx/contacts/:id", proxy.GetContact)
	mux.Handle("POST", "/mx/:mx/calls", proxy.PostMakeCall)
	mux.Handle("POST", "/mx/:mx/calls/:id", proxy.PostSIPAnswer)
	mux.Handle("GET", "/mx/:mx/calls/log", proxy.GetCallLog)
	mux.Handle("GET", "/mx/:mx/voicemails", proxy.GetVoiceMailList)
	mux.Handle("GET", "/mx/:mx/voicemails/:id", proxy.GetVoiceMailFile)
	mux.Handle("DELETE", "/mx/:mx/voicemails/:id", proxy.DeleteVoiceMail)
	mux.Handle("PATCH", "/mx/:mx/voicemails/:id", proxy.PatchVoiceMail)
	mux.Handle("POST", "/mx/:mx/tokens/:type/:topic", proxy.AddToken)

	StartHTTPServer(mux, host)            // запускаем HTTP сервер
	monitorSignals(os.Interrupt, os.Kill) // ожидаем сигнала остановки
}

// monitorSignals запускает мониторинг сигналов и возвращает значение, когда
// получает сигнал. В качестве параметров передается список сигналов, которые
// нужно отслеживать.
func monitorSignals(signals ...os.Signal) os.Signal {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, signals...)
	return <-signalChan
}

// StartHTTPServer запускает HTTP сервер.
func StartHTTPServer(mux http.Handler, hosts ...string) {
	if len(hosts) == 0 {
		return
	}
	// инициализируем HTTP сервер
	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Minute * 5,
	}
	// добавляем автоматическую поддержку TLS сертификатов для сервиса
	if !strings.HasPrefix(hosts[0], "localhost") &&
		!strings.HasPrefix(hosts[0], "127.0.0.1") {
		manager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(hosts...),
			Email:      "dmitrys@xyzrd.com",
			Cache:      autocert.DirCache("letsEncript.cache"),
		}
		server.TLSConfig = &tls.Config{
			GetCertificate: manager.GetCertificate,
		}
		server.Addr = ":https"
	} else if len(hosts) > 1 {
		server.Addr = ":http"
	} else {
		server.Addr = hosts[0]
	}
	// запускаем HTTP сервер
	go func() {
		var secure = (server.Addr == ":https" || server.Addr == ":443")
		slog := log.WithFields(log.Fields{
			"address": server.Addr,
			"tls":     secure,
		})
		if server.Addr != hosts[0] {
			slog = slog.WithField("host", strings.Join(hosts, ","))
		}
		slog.Info("http server")
		var err error
		if secure {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}
		if err != nil {
			log.WithError(err).Error("http server stoped")
			os.Exit(2)
		}
	}()
}
