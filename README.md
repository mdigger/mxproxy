# MXProxy

Данный сервис позволяет проксировать некоторые запросы к серверам Zultys MX и выполнять их в виде HTTP-запросов.

## API

В пути обращения к сервису требуется указывать уникальный идентификатор сервера MX: `/mx/<mx-id>/...`

Все запросы к сервису MXProxy **требуют авторизации** пользователя Zultys MX. Данные для авторизации передаются в заголовке HTTP Authorization (**Basic**).

Все ответы с кодом `2xx` должны рассматриваться как успешные, а `4xx` и `5xx` как неуспешные.

### Получение адресной книги

```http
GET /mx/<mx-id>/contacts
Authorization: Basic ZG06Nzg1NjE=
```

Возвращает серверную адресную книгу с пользователями MX:

```json
{
    "addressbook": [
        {
            "jid": 43884852633771555,
            "firstName": "SMS",
            "lastName": "Gateway C73",
            "extension": "3010"
        },
        {
            "jid": 43884851428118509,
            "firstName": "Peter",
            "lastName": "Hyde",
            "extension": "3044",
            "homePhone": "+1-202-555-0104",
            "cellPhone": "+1-512-555-0136",
            "email": "peterh@xyzrd.com",
            "did": "15125550136"
        },
        {
            "jid": 43884850646482261,
            "firstName": "Mike",
            "lastName": "Flynn",
            "extension": "3055",
            "homePhone": "+1-202-555-0104",
            "cellPhone": "+1-512-555-0136",
            "email": "mikef@xyzrd.com"
        },
        {
            "jid": 43884851147406145,
            "firstName": "Dmitry",
            "lastName": "Sedykh",
            "extension": "3095",
            "cellPhone": "+79031744445",
            "email": "dmitrys@xyzrd.com"
        },
        {
            "jid": 43884851851343044,
            "firstName": "Sergey",
            "lastName": "Kananykhin",
            "extension": "3096"
        },
        {
            "jid": 43884851324615074,
            "firstName": "John",
            "lastName": "Smith",
            "extension": "3098",
            "cellPhone": "12035160992",
            "did": "12035160992"
        },
        {
            "jid": 43884852031096113,
            "firstName": "Maxim",
            "lastName": "Donchenko",
            "extension": "3099",
            "cellPhone": "+420720961083"
        }
    ]
}
```

У контакта поддерживаются следующие поля: `jid`, `firstName`, `lastName`, `extension`, `homePhone`, `cellPhone`, `email`, `homeSystem`, `did`, `exchangeId`. Поля с пустыми значениями могут быть опущены.

### Поиск контакта

```http
GET /mx/<mx-id>/contacts/<contact-id>
Authorization: Basic ZG06Nzg1NjE=
```

Возвращает информацию о контакте. В качестве идентификатора контакта можно использовать его уникальный идентификатор (`jid`), внутренний номер (`extension`) или `email` адрес. Если ничего не найдено, то возвращается ошибка `404`.

```json
{
    "contact": {
        "jid": 43884852633771555,
        "firstName": "SMS",
        "lastName": "Gateway C73",
        "extension": "3010"
    }
}
```

### Вызов обратного звонка

```http
POST /mx/<mx-id>/calls
Authorization: Basic ZG06Nzg1NjE=
Content-Type: application/json; charset=utf-8

{
    "from":"79031744445",
    "to":"79031744437",
    "ringDelay":1,
    "vmDelay":30
}
```

Обязательными параметрами являются только `from` и `to`. `ringDelay` имеет значение по умолчанию `1`, а `vmDelay` - `30`.

В ответ возвращается информация о звонке:

```json
{
    "call": {
        "callId": 186,
        "deviceId": "3095",
        "calledDevice": "79031744437"
    }
}
```

### Ответ на звонок по SIP

```http
POST /mx/<mx-id>/calls/<call-id>
Authorization: Basic ZG06Nzg1NjE=
Content-Type: application/json; charset=utf-8

{
  "deviceId": "1099",
  "sipName": "maximd",
  "timeout": 30
}
```

Идентификатор звонка (`callId`) указывается в пути запроса. В теле передается идентификатор устройства (`deviceId`), имя устройства для SIP (`sipName`) и время ожидания ответа в секундах (`timeout`).

В ответ возвращается код `204` при успешном звонке, `400` при неудаче назначения умени устройства или `408` при превышении времени ожидания ответа.

### Лог звонков пользователя

```http
GET /mx/<mx-id>/calls/log?timestam=2017-08-20T00:00:00Z
Authorization: Basic ZG06Nzg1NjE=
```

Параметр `timestam` является не обязательным, но может ограничивать вывод в лог только тех звонков, которые были совершены после указанных даты и времени. Можно использовать как текстовое представление времени (`2017-08-20T00:00:00Z`), так и числовое (`1502625652`).

В ответ возвращается список звонков:

```json
{
  "calllog": [
    {
        "direction": "outgoing",
        "record_id": 961,
        "gcid": "63022-00-0000D-3AD",
        "disconnectTimestamp": 1503203445,
        "callingPartyNo": "79031744445",
        "originalCalledPartyNo": "79031744437",
        "callType": 1,
        "legType": 1,
        "selfLegType": 1
    },
    {
        "direction": "outgoing",
        "record_id": 962,
        "gcid": "63022-00-0000D-3AE",
        "disconnectTimestamp": 1503204267,
        "callingPartyNo": "79031744445",
        "originalCalledPartyNo": "79031744437",
        "callType": 1,
        "legType": 1,
        "selfLegType": 1
    },
    {
        "missed": true,
        "direction": "incoming",
        "record_id": 964,
        "gcid": "63022-00-0000D-3B0",
        "connectTimestamp": 1503204542,
        "disconnectTimestamp": 1503204547,
        "callingPartyNo": "3044",
        "originalCalledPartyNo": "3095",
        "firstName": "Peter",
        "lastName": "Hyde",
        "extension": "3044",
        "callType": 1,
        "legType": 1,
        "selfLegType": 9
    },
    {
        "missed": true,
        "direction": "incoming",
        "record_id": 967,
        "gcid": "63022-00-0000D-3B3",
        "connectTimestamp": 1503204601,
        "disconnectTimestamp": 1503204603,
        "callingPartyNo": "3044",
        "originalCalledPartyNo": "3095",
        "firstName": "Peter",
        "lastName": "Hyde",
        "extension": "3044",
        "callType": 1,
        "legType": 1,
        "selfLegType": 9
    }
  ]
}
```

Полный список возможных полей: `missed` _(bool)_, `direction`, `record_id` _(number)_, `gcid`, `connectTimestamp` _(timestamp)_, `disconnectTimestamp`_(timestamp)_, `callingPartyNo`, `originalCalledPartyNo`, `firstName`, `lastName`, `extension`, `serviceName`, `serviceExtension`, `callType` _(number)_, `legType` _(number)_, `selfLegType` _(number)_, `monitorType` _(number)_. Пустые поля могут быть опущены.

В случае отдачи пустого списка может быть небольшая задержка с ответом до 2-5 секунд.

### Список голосовой почты

```http
GET /mx/<mx-id>/voicemails
Authorization: Basic ZG06Nzg1NjE=
```

Возвращает список голосовой почты пользователя:

```json
{
    "voicemails": [
        {
            "from": "3044",
            "fromName": "Peter Hyde",
            "callerName": "Peter Hyde",
            "to": "3095",
            "ownerType": "user",
            "id": "68",
            "received": 1501545174,
            "duration": 2,
            "readed": true,
            "note": "TsHroN7KrMH78wQ7s48iHGdbmidNGFNd"
        },
        {
            "from": "3095",
            "fromName": "Dmitry Sedykh",
            "to": "3095",
            "ownerType": "user",
            "id": "82",
            "received": 1502213652,
            "duration": 29,
            "readed": true,
            "note": "text"
        },
        {
            "from": "3044",
            "fromName": "Peter Hyde",
            "callerName": "Peter Hyde",
            "to": "3095",
            "ownerType": "user",
            "id": "117",
            "received": 1502565440,
            "duration": 7
        }
    ]
}
```

Поддерживаемые поля: `from`, `fromName`, `callerName`, `to`, `ownerType`, `id`, `received` _(timestamp)_, `duration` _(number)_, `readed` _(bool)_, `note`. Пустые поля могут быть опущены.

### Файл с голосовым сообщением

```http
GET /mx/<mx-id>/voicemails/<id>
Authorization: Basic ZG06Nzg1NjE=
```

В ответ возвращается содержимое файла. В заголовке ответа передается его тип и имя файла:

```http
HTTP/1.1 200 OK
Content-Disposition: attachment; filename="u00043884851147406145/m0016.wav"
Content-Type: audio/wave

RIFF\WAVEfmt
...
```

Если голосовой файл с таким идентификатором не найден или он принадлежит другому пользователю, то возвращается ошибка `404`.

### Удаление голосовой почты

```http
DELETE /mx/<mx-id>/voicemails/<id>
Authorization: Basic ZG06Nzg1NjE=
```

В случае успешного удаления файла возвращается код `204`. Если голосовой файл с таким идентификатором не найден или он принадлежит другому пользователю, то возвращается ошибка `404`.

### Добавление заметки и установка флага прочитанности

```http
PATCH /mx/<mx-id>/voicemails/<id>
Authorization: Basic ZG06Nzg1NjE=

{
  "note": "текст заметки",
  "readed": true
}
```

Параметры `note` и `readed` являются не обязательными и их можно не отдавать, если планируется изменить толкьо что-то одно. Но **хотя бы один** из этих параметров должен быть указан обязательно. Или оба.

В случае успешного выполнения возвращается код `204`. Если голосовой файл с таким идентификатором не найден или он принадлежит другому пользователю, то возвращается ошибка `404`.

### Регистрация токена устройства

```http
POST /mx/<mx-id>/tokens/<type>/<app-id>
Authorization: Basic ZG06Nzg1NjE=

{
  "token": "aabb010203040506070809aabb"
}
```

`<type>` должен быть либо `apn`, либо `fcm` - в зависимости от типа токена. `<app-id>` задает идентификатор приложения для Android или идентификатор сертификата для Apple. Так же для устройств Apple может быть добавлен дополнительный параметр в адресе запроса `sandbox`, который указывает, что данный токен устройства может быть использован для отправки уведомлений через Apple Push Notification Sandbox:

```http
POST /mx/<mx-id>/tokens/apn/com.xyzrd.vialer.voip?sandbox
Authorization: Basic ZG06Nzg1NjE=

{
  "token": "aabb010203040506070809aabb"
}
```

## События

При поступлении информации о входящем звонке на отслеживаемый номер пользователя автоматически отправляется push-уведомление на все токены устройств, зарегистрированные в сервисе. Пример формата сообщения:

```json
{
    "callId": 190,
    "deviceId": "3095",
    "globalCallId": "2808630435142226873",
    "callingDevice": "3044",
    "calledDevice": "3095",
    "alertingDevice": "3095",
    "lastRedirectionDevice": "3044",
    "localConnectionInfo": "alerting",
    "cause": "normal",
    "callTypeFlags": 170917889,
    "timestamp": 1502213652
}
```

Некоторые пустые поля могут быть опущены.

## Конфигурационный файл

```json
{
    "mx": {
        "localhost": {
            "login": "server-login",
            "password": "12345678"
        }
    },
    "timeouts": {
        "connectTimeout": "5s",
        "readTimeout": "2s",
        "keepAliveDuration": "1m",
        "reconnectDelay": "1m"
    },
    "apn": {
        "push.p12": "password"
    },
    "fcm": {
        "android-app-id": "AAAA0bHpCVQ:-FCM-APP-KEY"
    }
}
```

Список сервером Zultys MX, поддерживаемых сервисом, задается в разделе `mx` в виде названия или адреса хоста сервера и логина с паролем для серверной авторизации. Поддерживается только защищенное соединение с сервером MX. По умолчанию, если порт не указан, используется порт 7778.

Не обязательные параметры `timeouts` позволяют задать максимальное время ожидание подключения (`connectTimeout`) к серверу MX, время ожидания ответа по умолчанию (`readTimeout`), интервал между пактетами keepAlive (`keepAliveDuration`) и задержку перед восстановление подключения к серверу (`reconnectDelay`).

Список сертификатов для Apple Push Notification задается в разделе `apn` в виде имени файла и пароля для его чтения.

Для Firebase Cloud Messages задается название приложения и ключ для отправки уведомлений.