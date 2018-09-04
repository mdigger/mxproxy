package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	app "github.com/mdigger/app-info"
	"github.com/mdigger/log"
	"github.com/mdigger/rest"
)

// информация о сервисе и версия
var (
	appName = "MXProxy" // название сервиса
	version = "2.3"     // версия
	date    string      // дата сборки
	commit  string      // версия git

	agent        = fmt.Sprintf("%s/%s", appName, version)
	lowerAppName = strings.ToLower(appName)
	// host         = lowerAppName + ".connector73.net" // имя сервера
	host       = "localhost:8080"
	configName = lowerAppName + ".toml" // имя файла с хранилищем токенов
	logFile    = filepath.Join("/var/log", lowerAppName+".log")
)

func main() {
	// инициализируем разбор параметров запуска сервиса
	var httphost = flag.String("port", app.Env("PORT", ":8080"),
		"http server `port`")
	var letsencrypt = flag.String("letsencrypt", app.Env("LETSENCRYPT_HOST", ""),
		"domain `host` name")
	flag.StringVar(&configName, "config", configName, "configuration `filename`")
	flag.Parse()
	// выводим в лог информацию о версии сервиса
	app.Parse(appName, version, commit, date)
	log.Info("service", app.LogInfo())

	// разбираем имя хоста и порт, на котором будет слушать веб-сервер
	port, err := app.Port(*httphost)
	if err != nil {
		log.Error("http host parse error", err)
		os.Exit(2)
	}

	// инициализируем сервис
	proxy, err := InitProxy()
	if err != nil {
		log.Error("initializing proxy error", "error", err)
		os.Exit(2)
	}
	defer proxy.Close()

	if proxy.adminWeb != "" {
		// запускаем административный веб
		var muxAdmin = &rest.ServeMux{
			Headers: map[string]string{
				"Server": agent, // ¯\_(ツ)_/¯
			},
			Logger: log.New("http admin"),
		}
		muxAdmin.Handles(rest.Paths{
			// отдает список запущенных соединений
			"/connections": rest.Methods{
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
			"/apps": rest.Methods{
				"GET": func(c *rest.Context) error {
					var list = make(map[string]string, len(proxy.appsAuth))
					for appName, secret := range proxy.appsAuth {
						list[appName] = secret
					}
					return c.Write(rest.JSON{"apps": list})
				},
			},
			// список зарегистрированных пользователей
			"/users": rest.Methods{
				"GET": func(c *rest.Context) error {
					return c.Write(
						rest.JSON{"users": proxy.store.section(bucketUsers)})
				},
				"POST": func(c *rest.Context) error {
					var login = c.Form("login")
					// останавливаем соединение
					if conn, ok := proxy.conns.Load(login); ok {
						proxy.conns.Delete(login) // удаляем из списка
						conn.(*MXConn).Close()    // останавливаем соединение
					}
					// удаляем из хранилища
					if err = proxy.store.RemoveUser(login); err != nil {
						return err
					}
					log.Info("mx user disconnected", "login", login)
					return c.Write(rest.JSON{"userLogout": login})
				},
			},
			// список зарегистрированных токенов устройств
			"/tokens": rest.Methods{
				"GET": func(c *rest.Context) error {
					return c.Write(
						rest.JSON{"tokens": proxy.store.section(bucketTokens)})
				},
			},
			// "/log": rest.Methods{
			// 	"GET": rest.File(logFile),
			// },
		})
		var serverAdmin = &http.Server{
			Addr:         proxy.adminWeb,
			Handler:      muxAdmin,
			ReadTimeout:  time.Second * 10,
			WriteTimeout: time.Minute * 5,
			ErrorLog:     log.StdLog(log.WARN, "http admin"),
		}
		log.Info("starting admin http server", "address", serverAdmin.Addr)
		go serverAdmin.ListenAndServe()
	}

	// инициализируем обработку HTTP запросов
	var httplogger = log.New("http")
	// инициализируем обработку HTTP запросов
	var mux = &rest.ServeMux{
		Headers: map[string]string{
			"Server": agent, // ¯\_(ツ)_/¯
		},
		Logger: httplogger,
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
	mux.Handle("GET", "/calls/:id", proxy.CallInfo)
	mux.Handle("PUT", "/calls/:id", proxy.SIPAnswer)
	mux.Handle("POST", "/calls/:id", proxy.Transfer)
	mux.Handle("DELETE", "/calls/:id", proxy.ClearConnection)
	mux.Handle("PATCH", "/calls/:name", proxy.AssignDevice)
	mux.Handle("PUT", "/calls/:id/hold", proxy.CallHold)
	mux.Handle("PUT", "/calls/:id/unhold", proxy.CallUnHold)
	mux.Handle("POST", "/calls/:id/record", proxy.CallRecording)
	mux.Handle("POST", "/calls/:id/record/stop", proxy.CallRecordingStop)
	mux.Handle("POST", "/calls/:id/conference", proxy.ConferenceCreateFromCall)

	mux.Handle("GET", "/voicemails", proxy.Voicemails)
	mux.Handle("GET", "/voicemails/:id", proxy.GetVoiceMailFile)
	mux.Handle("DELETE", "/voicemails/:id", proxy.DeleteVoicemail)
	mux.Handle("PATCH", "/voicemails/:id", proxy.PatchVoiceMail)

	mux.Handle("GET", "/conferences", proxy.ConferenceList)
	mux.Handle("POST", "/conferences", proxy.ConferenceCreate)
	mux.Handle("PUT", "/conferences/:id", proxy.ConferenceUpdate)
	mux.Handle("POST", "/conferences/:id", proxy.ConferenceJoin)
	mux.Handle("DELETE", "/conferences/:id", proxy.ConferenceDelete)
	mux.Handle("GET", "/conferences/info", proxy.ConferenceInfo)

	mux.Handle("PUT", "/tokens/:type/:topic/:token", proxy.Token)
	mux.Handle("DELETE", "/tokens/:type/:topic/:token", proxy.Token)

	mux.Handles(rest.Paths{
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
	var server = &http.Server{
		Addr:         port,
		Handler:      mux,
		ReadTimeout:  time.Second * 10,
		WriteTimeout: time.Second * 20,
		ErrorLog:     httplogger.StdLog(log.ERROR),
	}
	var hosts []string
	// настраиваем автоматическое получение сертификата
	if *letsencrypt != "" {
		hosts = strings.Split(*letsencrypt, ",")
		server.TLSConfig = app.LetsEncrypt(hosts...)
		server.Addr = ":443" // подменяем порт на 443
	} else {
		tlsConfig, err := app.LoadCertificates(filepath.Join(".", "certs"))
		if err != nil {
			httplogger.Error("certificates error", err)
			os.Exit(2)
		}
		if tlsConfig != nil {
			server.TLSConfig = tlsConfig
			hosts = make([]string, 0, len(tlsConfig.NameToCertificate))
			for name := range tlsConfig.NameToCertificate {
				hosts = append(hosts, name)
			}
		}
	}

	// отслеживаем сигнал о прерывании и останавливаем по нему сервер
	go func() {
		var sigint = make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint
		if err := server.Shutdown(context.Background()); err != nil {
			httplogger.Error("server shutdown", err)
		}
	}()
	// добавляем в статистику и выводим в лог информацию о запущенном сервере
	if server.TLSConfig != nil {
		// добавляем заголовок с обязательством использования защищенного
		// соединения в ближайший час
		mux.Headers["Strict-Transport-Security"] = "max-age=3600"
	}
	httplogger.Info("server",
		"listen", server.Addr,
		"tls", server.TLSConfig != nil,
		"hosts", hosts,
		"letsencrypt", *letsencrypt != "",
	)
	defer log.Info("service stoped")

	// в зависимости от того, поддерживаются сертификаты или нет, запускается
	// разная версию веб-сервера
	if server.TLSConfig != nil {
		err = server.ListenAndServeTLS("", "")
	} else {
		err = server.ListenAndServe()
	}
	if err != http.ErrServerClosed {
		httplogger.Error("server", err)
	} else {
		httplogger.Info("server stopped")
	}
}
