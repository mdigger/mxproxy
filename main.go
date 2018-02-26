package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
	// host         = lowerAppName + ".connector73.net" // имя сервера
	host       = "localhost:8080"
	configName = lowerAppName + ".toml" // имя файла с хранилищем токенов
	logFile    = filepath.Join("/var/log", lowerAppName+".log")
)

func init() {
	// инициализируем разбор параметров запуска сервиса
	flag.StringVar(&host, "host", host, "main server `host name`")
	flag.StringVar(&configName, "config", configName, "configuration `filename`")
	flag.Var(log.Flag(), "log", "log `level`")
	flag.Parse()

	// выводим информацию о текущей версии
	var verInfoFields = log.With(
		"name", appName,
		"version", version)
	if date != "" {
		verInfoFields = verInfoFields.With("builded", date)
	}
	if git != "" {
		verInfoFields = verInfoFields.With("git", git)
		agent += " (" + git + ")"
	}
	if host, err := os.Hostname(); err == nil {
		verInfoFields = verInfoFields.With("host", host)
	}
	verInfoFields.Info("service info")
	log.Info("log", "level", log.Flag().String())
}

func main() {
	// инициализируем сервис
	proxy, err := InitProxy()
	if err != nil {
		log.Error("initializing proxy error", "error", err)
		os.Exit(2)
	}
	defer proxy.Close()
	// инициализируем обработку HTTP запросов
	var mux = &rest.ServeMux{
		Headers: map[string]string{
			"Server": agent, // ¯\_(ツ)_/¯
		},
		Logger: log.New("http"),
	}
	// генерация авторизационных токенов
	mux.Handle("POST", "/auth", proxy.Login)
	mux.Handle("GET", "/auth", proxy.LoginInfo)
	mux.Handle("DELETE", "/auth", proxy.Logout)

	mux.Handle("GET", "/contacts", proxy.Contacts)
	mux.Handle("GET", "/services", proxy.Services)

	mux.Handle("GET", "/calls", proxy.CallLog)
	mux.Handle("PATCH", "/calls", proxy.SetMode)
	mux.Handle("POST", "/calls", proxy.MakeCall)
	mux.Handle("PUT", "/calls/:id", proxy.SIPAnswer)
	mux.Handle("POST", "/calls/:id", proxy.Transfer)
	mux.Handle("DELETE", "/calls/:id", proxy.ClearConnection)
	mux.Handle("PATCH", "/calls/:name", proxy.AssignDevice)
	mux.Handle("PUT", "/calls/:id/hold", proxy.CallHold)
	mux.Handle("PUT", "/calls/:id/unhold", proxy.CallUnHold)

	mux.Handle("GET", "/voicemails", proxy.Voicemails)
	mux.Handle("GET", "/voicemails/:id", proxy.GetVoiceMailFile)
	mux.Handle("DELETE", "/voicemails/:id", proxy.DeleteVoicemail)
	mux.Handle("PATCH", "/voicemails/:id", proxy.PatchVoiceMail)

	mux.Handle("GET", "/conferences", proxy.ConferenceList)
	mux.Handle("POST", "/conferences", proxy.ConferenceCreate)
	mux.Handle("DELETE", "/conferences/:id", proxy.ConferenceDelete)

	mux.Handle("PUT", "/tokens/:type/:topic/:token", proxy.Token)
	mux.Handle("DELETE", "/tokens/:type/:topic/:token", proxy.Token)

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
		"/debug/log": rest.Methods{
			"GET": rest.File(logFile),
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
	startHTTPServer(mux, host) // запускаем HTTP сервер

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
func startHTTPServer(mux http.Handler, host string) {
	// инициализируем HTTP сервер
	var server = &http.Server{
		Handler:      mux,
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Minute * 5,
		ErrorLog:     log.StdLog(log.WARN, "http"),
	}
	// анализируем порт
	var httphost, port, err = net.SplitHostPort(host)
	if err, ok := err.(*net.AddrError); ok && err.Err == "missing port in address" {
		httphost = err.Addr
	}
	var isIP = (net.ParseIP(httphost) != nil)
	var notLocal = (httphost != "localhost" &&
		!strings.HasSuffix(httphost, ".local") &&
		!isIP)
	var canCert = notLocal && httphost != "" &&
		(port == "443" || port == "https" || port == "")

	// добавляем автоматическую поддержку TLS сертификатов для сервиса
	if canCert {
		manager := autocert.Manager{
			Prompt: autocert.AcceptTOS,
			HostPolicy: func(_ context.Context, host string) error {
				if host != httphost {
					log.Error("unsupported https host", "host", host)
					return errors.New("acme/autocert: host not configured")
				}
				return nil
			},
			Email: "dmitrys@xyzrd.com",
			Cache: autocert.DirCache("letsEncrypt.cache"),
		}
		server.TLSConfig = &tls.Config{
			GetCertificate: manager.GetCertificate,
		}
		server.Addr = ":https"
		// поддержка получения сертификата Let's Encrypt
		go http.ListenAndServe(":http", manager.HTTPHandler(nil))
	} else if port == "" {
		server.Addr = net.JoinHostPort(httphost, "http")
	} else {
		server.Addr = net.JoinHostPort(httphost, port)
	}
	// запускаем HTTP сервер
	go func() {
		log.Info("starting http server",
			"address", server.Addr, "tls", canCert, "host", httphost)
		var err error
		if canCert {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}
		if err != nil {
			log.Error("http server stopped", "error", err)
			os.Exit(2)
		}
	}()
}
