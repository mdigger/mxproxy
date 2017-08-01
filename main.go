package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/mdigger/csta"
	"github.com/mdigger/log"
	"github.com/mdigger/rest"
)

var (
	appName = "mxproxy"                 // название сервиса
	version = "1.0.5"                   // версия
	date    = "2017-08-01"              // дата сборки
	host    = "mxproxy.connector73.net" // имя сервера
	debug   = false                     // флаг вывода отладочной информации
)

var (
	storeDB     *Store                  // хранилище токенов устройств
	apnsClients map[string]*http.Client // список клиентов для пуш по bundleID
	gfcmKeys    map[string]string       // список ключей для пушей Google
	mxlist      map[string]*MX          // список запущенных соединений с MX
)

func main() {
	// инициализируем разбор параметров запуска сервиса
	configName := appName + ".json"
	tokensDBName := appName + ".db"
	flag.StringVar(&host, "host", host, "main server `name`")
	flag.StringVar(&configName, "config", configName, "config `filename`")
	flag.StringVar(&tokensDBName, "db", tokensDBName, "tokens DB `filename`")
	flag.BoolVar(&debug, "debug", debug, "debug output")
	var cstaOutput bool
	flag.BoolVar(&cstaOutput, "csta", cstaOutput, "CSTA output")
	var logFlags int = log.Lindent
	flag.IntVar(&logFlags, "logflag", logFlags, "log flags")
	flag.Parse()

	log.SetFlags(logFlags)
	log.WithFields(log.Fields{
		"version": version,
		"build":   date,
		"name":    appName,
	}).Info("starting service")

	// разрешаем вывод отладочной информации, включая вывод команд CSTA
	if debug {
		log.SetLevel(log.DebugLevel)
	}
	if cstaOutput {
		csta.SetLogOutput(os.Stdout)
		csta.SetLogFlags(0)
	}

	// читаем конфигурационный файл сервиса
	log.WithField("file", configName).Info("loading config")
	file, err := os.Open(configName)
	if err != nil {
		log.WithError(err).Error("error reading config")
		os.Exit(2)
	}
	var config = new(Config)
	err = json.NewDecoder(file).Decode(config)
	file.Close()
	if err != nil {
		log.WithError(err).Error("error parsing config")
		os.Exit(2)
	}

	// загружаем и разбираем сертификаты для работы с Apple Push
	apnsClients = make(map[string]*http.Client)
	for name, password := range config.APNS {
		cert, err := LoadAPNSCertificate(name, password)
		if err != nil {
			log.WithError(err).Error("error reading apns cert")
			os.Exit(3)
		}
		if cert.Production {
			apnsClients[cert.BundleID] = cert.Client
		}
		// для поддержки sandbox добавляем к bundleID символ '~'
		if cert.Development {
			apnsClients[cert.BundleID+"~"] = cert.Client
		}
		log.WithFields(log.Fields{
			"bundle":     cert.BundleID,
			"expire":     cert.Expire.Format(time.RFC822),
			"sandbox":    cert.Development,
			"production": cert.Production,
		}).Info("apns cert")
	}
	// добавляем ключи для Google Firebase Cloud Messages.
	gfcmKeys = config.GFCM

	// открываем хранилище с токенами устройств для пушей
	log.WithField("file", tokensDBName).Info("opening tokens db")
	storeDB, err = OpenStore(tokensDBName)
	if err != nil {
		log.WithError(err).Error("error opening db")
		os.Exit(4)
	}
	defer storeDB.Close()

	// инициализируем обработку HTTP запросов
	var mux = &rest.ServeMux{
		Headers: map[string]string{
			"Server":            "MXProxy/1.0",
			"X-API-Version":     "1.0",
			"X-Service-Version": version,
		},
		Logger: log.WithField("type", "http"),
	}

	// инициализируем серверные соединения с MX серверами
	var stoping bool // флаг остановки сервиса
	mxlist = make(map[string]*MX, len(config.MXList))
	for name, mxConfig := range config.MXList {
		mx := &MX{MXConfig: mxConfig} // инициализируем конфигурацию
		ctxlog := log.WithField("name", name)
		// подключаемся к серверу
		ctxlog.WithFields(log.Fields{
			"addr":  mxConfig.Addr,
			"login": mxConfig.Login,
		}).Info("connecting to mx")
		if err := mx.Connect(); err != nil {
			ctxlog.WithError(err).Error("error connecting to mx server")
			os.Exit(5)
		}
		ctxlog = ctxlog.WithField("mx", mx.SN)
		// в случае дублирования серверов в конфигурации, выходим с ошибкой
		if _, ok := mxlist[mx.SN]; ok {
			ctxlog.Error("duplicate mx server")
			os.Exit(5)
		}
		// сохраняем в списке запущенных соединений под уникальным
		// идентификатором сервера MX.
		mxlist[mx.SN] = mx
		ctxlog.Debug("connected to mx")

		// добавляем поддержку HTTP обработчиков для каждого MX
		prefix := fmt.Sprintf("/mx/%s/", name)
		mux.Handle("GET", prefix+"addressbook", mx.GetAddressBook)
		mux.Handle("GET", prefix+"calllog", mx.GetCallLog)
		mux.Handle("POST", prefix+"call", mx.PostCall)
		mux.Handle("POST", prefix+"token/:type/:bundle", mx.AddToken)

		// запускаем мониторинг ответов сервера MX
		go func(log *log.Context) {
		monitoring:
			err := mx.Monitoring()
			if stoping {
				return
			}
			mx.Close()
			log.WithError(err).Error("monitoring error")
		connecting:
			time.Sleep(time.Second * 10)
			log.Info("reconnecting to mx...")
			if err := mx.Connect(); err != nil {
				log.WithError(err).Error("error connecting to mx server")
				goto connecting
			}
			log.Debug("reconnected to mx")
			goto monitoring
		}(ctxlog)
		// добавляем всех мониторинг входящих звонков для всех
		// зарегистрированных пользователей
	}

	if debug {
		// добавляем для отладки вывод хранилище в виде JSON
		var path = "/db"
		mux.Handle("GET", path, func(c *rest.Context) error {
			dbjson, err := storeDB.json()
			if err != nil {
				return err
			}
			return c.Write(dbjson)
		})
	}

	// инициализируем HTTP сервер
	server := &http.Server{
		Addr:         host,
		Handler:      mux,
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Second * 20,
	}
	// добавляем автоматическую поддержку TLS сертификатов для сервиса
	if !strings.HasPrefix(host, "localhost") &&
		!strings.HasPrefix(host, "127.0.0.1") {
		manager := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(host),
			Email:      "dmitrys@xyzrd.com",
			Cache:      autocert.DirCache("letsEncript.cache"),
		}
		server.TLSConfig = &tls.Config{
			GetCertificate: manager.GetCertificate,
		}
		server.Addr = ":https"
	}
	// запускаем HTTP сервер
	go func() {
		var secure = (server.Addr == ":https" || server.Addr == ":443")
		slog := log.WithFields(log.Fields{
			"address": server.Addr,
			"https":   secure,
		})
		if server.Addr != host {
			slog = slog.WithField("host", host)
		}
		slog.Info("starting http server")
		if secure {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}
		if err != nil {
			log.WithError(err).Warning("http server stoped")
			os.Exit(3)
		}
	}()

	monitorSignals(os.Interrupt, os.Kill) // ожидаем сигнала остановки
	stoping = true                        // флаг планомерной остановки
	// // останавливаем все серверные соединения
	for _, mx := range mxlist {
		mx.Close()
	}
	log.Info("service stoped")
}

// monitorSignals запускает мониторинг сигналов и возвращает значение, когда
// получает сигнал. В качестве параметров передается список сигналов, которые
// нужно отслеживать.
func monitorSignals(signals ...os.Signal) os.Signal {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, signals...)
	return <-signalChan
}
