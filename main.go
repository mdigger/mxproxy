package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/mdigger/log"
	"github.com/mdigger/rest"
	"golang.org/x/crypto/acme/autocert"
)

// информация о сервисе и версия
var (
	appName = "MXProxy" // название сервиса
	version = "2.0"     // версия
	date    = ""        // дата сборки
	git     = ""        // версия git

	agent        = fmt.Sprintf("%s/%s", appName, version)
	lowerAppName = strings.ToLower(appName)
	host         = lowerAppName + ".connector73.net" // имя сервера
	configName   = lowerAppName + ".json"            // имя файла с хранилищем токенов
	cstaOutput   = false                             // флаг вывода команд и ответов CSTA
	debug        = false                             // флаг вывода отладочной информации
)

func init() {
	// инициализируем разбор параметров запуска сервиса
	flag.StringVar(&host, "host", host, "main server `host name`")
	flag.StringVar(&configName, "config", configName, "configuration `filename`")
	flag.BoolVar(&debug, "debug", debug, "debug output")
	var logFlags = log.Lindent | log.LstdFlags
	flag.IntVar(&logFlags, "logflag", logFlags, "log flags")
	flag.BoolVar(&cstaOutput, "csta", cstaOutput, "CSTA output")
	flag.Parse()

	log.SetFlags(logFlags) // устанваливаем флаги вывода в лог
	// разрешаем вывод отладочной информации, включая вывод команд CSTA
	if debug {
		log.SetLevel(log.DebugLevel)
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
		agent += " (" + git + ")"
	}
	log.WithFields(verInfoFields).Info("service info")
}

func main() {
	// инициализируем сервис
	proxy, err := InitProxy()
	if err != nil {
		log.WithError(err).Error("initializing proxy error")
		os.Exit(2)
	}
	defer proxy.Close()
	// инициализируем обработку HTTP запросов
	var mux = &rest.ServeMux{
		Headers: map[string]string{
			"Server": agent, // ¯\_(ツ)_/¯
		},
		Logger: log.WithField("ctx", "http"),
	}
	// генериция авторизационных токенов
	mux.Handle("POST", "/auth", proxy.Login)
	mux.Handle("GET", "/auth/logout", proxy.Logout)

	mux.Handle("GET", "/contacts", proxy.Contacts)

	mux.Handle("GET", "/calls", proxy.CallLog)
	mux.Handle("PATCH", "/calls", proxy.SetMode)
	mux.Handle("POST", "/calls", proxy.MakeCall)
	mux.Handle("PUT", "/calls/:id", proxy.SIPAnswer)

	mux.Handle("GET", "/voicemails", proxy.Voicemails)
	mux.Handle("GET", "/voicemails/:id", proxy.GetVoiceMailFile)
	mux.Handle("DELETE", "/voicemails/:id", proxy.DeleteVoicemail)
	mux.Handle("PATCH", "/voicemails/:id", proxy.PatchVoiceMail)

	mux.Handle("PUT", "/tokens/:type/:topic/:token", proxy.Token)
	mux.Handle("DELETE", "/tokens/:type/:topic/:token", proxy.Token)

	if debug {
		mux.Handles(rest.Paths{
			// отдает список запущенных соединений
			"/debug/connections": rest.Methods{
				"GET": func(c *rest.Context) error {
					var list []string
					proxy.conns.Range(func(login, _ interface{}) bool {
						list = append(list, login.(string))
						return true
					})
					sort.Strings(list)
					return c.Write(rest.JSON{"connections": list})
				},
			},
			// список зарегистрированных приложений для авторизации OAuth2
			"/debug/apps": rest.Methods{
				"GET": func(c *rest.Context) error {
					var list = make(map[string]string, len(proxy.appsAuth))
					for appName, secret := range proxy.appsAuth {
						list[appName] = secret
					}
					return c.Write(rest.JSON{"apps": list})
				},
			},
			// список зарегистрированных пользователей
			"/debug/users": rest.Methods{
				"GET": func(c *rest.Context) error {
					return c.Write(
						rest.JSON{"users": proxy.store.section(bucketUsers)})
				},
			},
			// список зарегистрированных токенов устройств
			"/debug/tokens": rest.Methods{
				"GET": func(c *rest.Context) error {
					return c.Write(
						rest.JSON{"tokens": proxy.store.section(bucketTokens)})
				},
			},
		}, func(c *rest.Context) error {
			// проверяем авторизацию при обращении к данным
			clientID, secret, ok := c.BasicAuth()
			if !ok {
				c.SetHeader("WWW-Authenticate",
					fmt.Sprintf("Basic realm=%q", appName+" client application"))
				return rest.ErrUnauthorized
			}
			if appSecret, ok := proxy.appsAuth[clientID]; !ok || appSecret != secret {
				return rest.ErrForbidden
			}
			c.AddLogField("app", clientID)
			return nil
		})
	}

	sendMonitorText("service started")
	defer sendMonitorText("service stopped")
	startHTTPServer(mux, host)            // запускаем HTTP сервер
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
func startHTTPServer(mux http.Handler, hosts ...string) {
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
		slog.Info("starting http server")
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
