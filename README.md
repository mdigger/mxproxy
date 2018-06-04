# MX Proxy 2.1

## Авторизация пользователя и приложения

Для авторизации используется протокол [Resource Owner Password Credentials Grant](https://tools.ietf.org/html/rfc6749#section-4.3).

```http
POST /auth HTTP/1.1
Authorization: Basic Y2xpZW50NzY2NDpNbXgwT2xXRURJRGhQR2dHcExRRnhCd3BDU1BwOUlJWQ==
Content-Type: application/x-www-form-urlencoded; charset=utf-8

username=dmitrys%40xyzrd.com&password=78561&grant_type=password
```

Логин пользователя указывается в `username`, пароль - в `password`. `grant_type` должен содержать строку `password`. В заголовке запроса передается HTTP Basic авторизация приложения (`client_id`:`secret`). Последние должны быть зарегистрированы в конфигурации сервера.

В ответ возвращается токен, который потом используется для обращения к другим функциям API:

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Server: MXProxy/2.0

{
    "token_type": "Bearer",
    "access_token": "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6Im92dGU0ZSJ9.eyJleHAiOjE1MDQ2MzI1MzgsImlhdCI6MTUwNDYyODkyOCwiaXNzIjoiaHR0cHM6Ly9sb2NhbGhvc3Q6ODAwMCIsImp0aSI6Ik5acGo2Q1MwIiwic3ViIjoiZG1pdHJ5c0B4eXpyZC5jb20ifQ.zXd9STR6CVuplm8jHoBG2mkB2TUaAdr2QJj_JKItqxbfSVivM5WalWX7Z6SrX_ANtqQSj3bGmW68GGg7_zal0Q",
    "expires_in": 3600
}
```

Токен действителен ограниченное количество времени, которое указывается в `expires_in` в секундах.

## Информация об авторизованном пользователе MX

```http
GET /auth HTTP/1.1
Authorization: Bearer <token>
```

Возвращает информацию об авторизованном пользователе:

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{
    "mx": "63022",
    "ext": "3044",
    "jid": "43884851428118509",
    "softPhonePwd": "EqNe45DYvZH2va1gesL7AA1ldtP04j4a"
}
```

## Удаление пользователя

```http
DELETE /auth/logout HTTP/1.1
Authorization: Bearer <token>
```

Останавливает соединение с сервером MX для данного пользователя и удаляет его из списка активных соединений.

## Список контактов

```http
GET /contacts HTTP/1.1
Authorization: Bearer <token>
```

Возвращает список контактов сервера MX. Контакты упорядочены по внутреннему номеру (`ext`):

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{
    "contacts": [
        {
            "jid": "43884852633771555",
            "firstName": "SMS",
            "lastName": "Gateway C73",
            "ext": "3010"
        },
        {
            "jid": "43884851338921185",
            "firstName": "mxflex",
            "lastName": "mxflex",
            "ext": "3042"
        },
        {
            "jid": "43884852654574210",
            "firstName": "Ilia",
            "lastName": "Test",
            "ext": "3043"
        },
        {
            "jid": "43884851428118509",
            "firstName": "Peter",
            "lastName": "Hyde",
            "ext": "3044",
            "homePhone": "+1-202-555-0104",
            "cellPhone": "+1-512-555-0136",
            "email": "peterh@xyzrd.com",
            "did": "15125550136"
        },
        {
            "jid": "43884850646482261",
            "firstName": "Mike",
            "lastName": "Flynn",
            "ext": "3055",
            "homePhone": "+1-202-555-0104",
            "cellPhone": "+1-512-555-0136",
            "email": "mikef@xyzrd.com"
        },
        {
            "jid": "43884850557879186",
            "firstName": "Test",
            "lastName": "One",
            "ext": "3080"
        },
        {
            "jid": "43884851776746473",
            "firstName": "Test",
            "lastName": "Two",
            "ext": "3081"
        },
        {
            "jid": "43884852542754454",
            "firstName": "Test",
            "lastName": "Three",
            "ext": "3082"
        },
        {
            "jid": "43884852535898307",
            "firstName": "dstest1",
            "lastName": "dstest1",
            "ext": "3091"
        },
        {
            "jid": "43884850939404214",
            "firstName": "dstest2",
            "lastName": "dstest2",
            "ext": "3092",
            "cellPhone": "16693507465",
            "did": "16693507465"
        },
        {
            "jid": "43884850647480796",
            "firstName": "Test",
            "lastName": "Admin",
            "ext": "3093"
        },
        {
            "jid": "43884852355777349",
            "firstName": "Zultys",
            "lastName": "Test",
            "ext": "3094"
        },
        {
            "jid": "43884851147406145",
            "firstName": "Dmitry",
            "lastName": "Sedykh",
            "ext": "3095",
            "cellPhone": "+79031744445",
            "email": "dmitrys@xyzrd.com"
        },
        {
            "jid": "43884851851343044",
            "firstName": "Sergey",
            "lastName": "Kananykhin",
            "ext": "3096"
        },
        {
            "jid": "43884851514905017",
            "firstName": "Test",
            "lastName": "Zultys",
            "ext": "3097"
        },
        {
            "jid": "43884851324615074",
            "firstName": "John",
            "lastName": "Smith",
            "ext": "3098",
            "cellPhone": "12035160992",
            "did": "12035160992"
        },
        {
            "jid": "43884852031096113",
            "firstName": "Maxim",
            "lastName": "Donchenko",
            "ext": "3099",
            "cellPhone": "+420720961083"
        }
    ]
}
```

У контакта поддерживаются следующие поля: `jid`, `firstName`, `lastName`, `ext`, `homePhone`, `cellPhone`, `email`, `homeSystem`, `did`, `exchangeId`. Поля с пустыми значениями могут быть опущены.

## Список звонков пользователя

```http
GET /calls?timestamp=1503223469 HTTP/1.1
Authorization: Bearer <token>
```

Возвращает лог пользовательских звонков, упорядоченный по идентификатору (`record_id`). Необязательный параметра в URL `timestamp` позволяет задать фильтрацию списка: выводиться будут только те звонки, которые были совершены после указанной даты. `timestamp` может быть указан как в числовом виде (`1503223469`), так и виде строки (`2017-09-03T00:00:00Z`).

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{
    "callLog": [
        {
            "missed": true,
            "direction": "incoming",
            "record_id": 973,
            "gcid": "63022-00-0000D-3B9",
            "connectTimestamp": 1503223469,
            "disconnectTimestamp": 1503223472,
            "callingPartyNo": "3044",
            "originalCalledPartyNo": "3095",
            "firstName": "Peter",
            "lastName": "Hyde",
            "ext": "3044",
            "callType": 1,
            "legType": 1,
            "selfLegType": 9
        },
        {
            "direction": "outgoing",
            "record_id": 975,
            "gcid": "63022-00-0000D-3BB",
            "disconnectTimestamp": 1503258153,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 976,
            "gcid": "63022-00-0000D-3BC",
            "disconnectTimestamp": 1503264934,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1047,
            "gcid": "63022-00-0000D-3F8",
            "disconnectTimestamp": 1503472515,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1138,
            "gcid": "63022-00-0000D-444",
            "disconnectTimestamp": 1503961073,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1139,
            "gcid": "63022-00-0000D-445",
            "disconnectTimestamp": 1503961572,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1140,
            "gcid": "63022-00-0000D-446",
            "disconnectTimestamp": 1503961749,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1141,
            "gcid": "63022-00-0000D-447",
            "disconnectTimestamp": 1503963185,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1142,
            "gcid": "63022-00-0000D-448",
            "disconnectTimestamp": 1503963904,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1143,
            "gcid": "63022-00-0000D-449",
            "disconnectTimestamp": 1503964356,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1158,
            "gcid": "63022-00-0000D-458",
            "disconnectTimestamp": 1504115834,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1159,
            "gcid": "63022-00-0000D-459",
            "disconnectTimestamp": 1504122163,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1169,
            "gcid": "63022-00-0000D-463",
            "disconnectTimestamp": 1504189334,
            "callingPartyNo": "3095",
            "originalCalledPartyNo": "3044",
            "firstName": "Peter",
            "lastName": "Hyde",
            "extension": "3044",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1186,
            "gcid": "63022-00-0000D-474",
            "disconnectTimestamp": 1504263779,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "+79031744445",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1187,
            "gcid": "63022-00-0000D-475",
            "disconnectTimestamp": 1504263810,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "+74992549993",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1188,
            "gcid": "63022-00-0000D-476",
            "disconnectTimestamp": 1504263972,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "+74992549993",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "missed": true,
            "direction": "incoming",
            "record_id": 1190,
            "gcid": "63022-00-0000D-478",
            "connectTimestamp": 1504522727,
            "disconnectTimestamp": 1504522740,
            "callingPartyNo": "3044",
            "originalCalledPartyNo": "3095",
            "firstName": "Peter",
            "lastName": "Hyde",
            "ext": "3044",
            "callType": 1,
            "legType": 1,
            "selfLegType": 9
        },
        {
            "direction": "incoming",
            "record_id": 1192,
            "gcid": "63022-00-0000D-478",
            "connectTimestamp": 1504522742,
            "disconnectTimestamp": 1504522747,
            "callingPartyNo": "3044",
            "originalCalledPartyNo": "voicemail.3095",
            "firstName": "Peter",
            "lastName": "Hyde",
            "ext": "3044",
            "callType": 1,
            "legType": 1,
            "selfLegType": 3
        },
        {
            "missed": true,
            "direction": "incoming",
            "record_id": 1193,
            "gcid": "63022-00-0000D-47A",
            "connectTimestamp": 1504523032,
            "disconnectTimestamp": 1504523045,
            "callingPartyNo": "3044",
            "originalCalledPartyNo": "3095",
            "firstName": "Peter",
            "lastName": "Hyde",
            "ext": "3044",
            "callType": 1,
            "legType": 1,
            "selfLegType": 9
        },
        {
            "direction": "incoming",
            "record_id": 1195,
            "gcid": "63022-00-0000D-47A",
            "connectTimestamp": 1504523045,
            "disconnectTimestamp": 1504523053,
            "callingPartyNo": "3044",
            "originalCalledPartyNo": "voicemail.3095",
            "firstName": "Peter",
            "lastName": "Hyde",
            "ext": "3044",
            "callType": 1,
            "legType": 1,
            "selfLegType": 3
        },
        {
            "missed": true,
            "direction": "incoming",
            "record_id": 1207,
            "gcid": "63022-00-0000D-486",
            "connectTimestamp": 1504524355,
            "disconnectTimestamp": 1504524367,
            "callingPartyNo": "3044",
            "originalCalledPartyNo": "3095",
            "firstName": "Peter",
            "lastName": "Hyde",
            "ext": "3044",
            "callType": 1,
            "legType": 1,
            "selfLegType": 9
        },
        {
            "direction": "incoming",
            "record_id": 1209,
            "gcid": "63022-00-0000D-486",
            "connectTimestamp": 1504524370,
            "disconnectTimestamp": 1504524373,
            "callingPartyNo": "3044",
            "originalCalledPartyNo": "voicemail.3095",
            "firstName": "Peter",
            "lastName": "Hyde",
            "ext": "3044",
            "callType": 1,
            "legType": 1,
            "selfLegType": 3
        },
        {
            "direction": "outgoing",
            "record_id": 1286,
            "gcid": "63022-00-0000D-4D4",
            "disconnectTimestamp": 1504626178,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1287,
            "gcid": "63022-00-0000D-4D5",
            "disconnectTimestamp": 1504626217,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1289,
            "gcid": "63022-00-0000D-4D7",
            "disconnectTimestamp": 1504626312,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1295,
            "gcid": "63022-00-0000D-4DD",
            "disconnectTimestamp": 1504626448,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        },
        {
            "direction": "outgoing",
            "record_id": 1301,
            "gcid": "63022-00-0000D-4E3",
            "disconnectTimestamp": 1504626848,
            "callingPartyNo": "79031744445",
            "originalCalledPartyNo": "79031744437",
            "callType": 1,
            "legType": 1,
            "selfLegType": 1
        }
    ]
}
```

Полный список возможных полей: `missed` (_bool_), `direction`, `record_id` (_number_), `gcid`, `connectTimestamp` (_timestamp_), `disconnectTimestamp` (_timestamp_), `callingPartyNo`, `originalCalledPartyNo`, `firstName`, `lastName`, `ext`, `serviceName`, `serviceExtension`, `callType` (_number_), `legType` (_number_), `selfLegType` (_number_), `monitorType` (_number_). Пустые поля могут быть опущены.

К сожалению, сервер MX не предоставляет возможности определить, что список отдан полностью, поэтому это делается чисто эвристическим способом: обычно ответ разбивается сервером на группы по 21 звонку, поэтому проверяется, что последняя группа содержит меньше 21 звонка. Если вдруг случается, что в последней группе ровно 21 звонок, то ответ придется ждать до окончания таймаута ожидания (2 минуты по умолчанию).

## Список сервисов

```http
GET /services HTTP/1.1
Authorization: Bearer <token>
```

В ответ возвращает список сервисов:

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Content-Length: 917

{
    "services": [
        {
            "id": "3502649365667727804",
            "name": "TESTIVR",
            "type": "AA",
            "ext": "3777"
        },
        {
            "id": "3458764515081031672",
            "name": "park",
            "type": "ParkServer",
            "ext": "*77"
        },
        {
            "id": "3458764513956069980",
            "name": "voicemail",
            "type": "VM",
            "ext": "*86"
        },
        {
            "id": "3502649365616396596",
            "name": "HoldForPush",
            "type": "AA",
            "ext": "3799"
        },
        {
            "id": "3458764515541333316",
            "name": "bind",
            "type": "BindServer",
            "ext": "*27"
        },
        {
            "id": "3502649364391454199",
            "name": "AAA_1",
            "type": "AdvancedACD",
            "ext": "3999"
        }
    ]
}
```

## Режим звонков

```http
PATCH /calls HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json; charset=utf-8

{"remote":true,"device":"79031744445"}
```

Устанавливает режим звонков (`remote` или `local`):

```json
{
  "remote": true,
  "deviceName": "phone",
  "ringDelay": 1,
  "vmDelay": 30
}
```

Опциональные параметры:

- `ringDelay` - если не указано, то `1`
- `vmDelay` - если не указано, то `30`

В ответ возвращается полный список параметров, использованных при установке режима:

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{
    "callMode": {
        "remote": true,
        "device": "79031744445",
        "ringDelay": 1,
        "vmDelay": 30
    }
}
```

## Назначение устройства для звонка

```http
PATCH /calls/{name} HTTP/1.1
Authorization: Bearer <token>
```

## Серверный звонок

```http
POST /calls HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json; charset=utf-8

{"from":"79031744445","to":"79031744437"}
```

Осуществляет серверный звонок с/на указанный в запросе номер:

```json
{
  "from": "79031744445",
  "to": "79031744437",
  "device": "dmitrys"
}
```

Опциональные параметры:

- если `device` указан, то происходит вызов назначения устройства.

В ответ возвращается информация о звонке:

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{
    "makeCall": {
        "callId": 123,
        "deviceId": "79031744445",
        "calledDevice": "79031744437"
    }
}
```

## Перехват звонка для SIP устройства

```http
PUT /calls/123 HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json; charset=utf-8

{"device":"sipPhone","timeout":30}
```

Позволяет перехватить ответ на звонок на SIP устройство:

```json
{
  "device": "sipPhone",
  "timeout": 30,
  "assign": true
}
```

Опциональные параметры:

- `timeout` по умолчанию равен `30`.
- если `assign` указан, то происходит вызов назначения устройства.

В ответ возвращается полный список параметров или ошибка, если не удалось перехватить звонок.

## Перенаправление звонка

```http
POST /calls/123 HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json; charset=utf-8

{"device":"sipPhone","to":"12345"}
```

Позволяет перенаправить звонок на другое устройство:

```json
{
  "callId": 123,
  "device": "sipPhone",
  "to": "12345"
}
```

В ответ возвращается полный список параметров или ошибка с таймаутом.

## Сброс звонка

```http
DELETE /calls/123 HTTP/1.1
Authorization: Bearer <token>
```

Позволяет сбросить звонок.

В ответ возвращается ответ с информацией о сброшенном звонке:

```json
{"connectionCleared": {...}}
```

## Блокировка звонка

```http
PUT /calls/123/hold HTTP/1.1
Authorization: Bearer <token>
```

Позволяет заблокировать звонок.

В ответ возвращается ответ с информацией о блокированном звонке:

```json
{"held": {...}}
```

## Разблокировка звонка

```http
PUT /calls/123/unhold HTTP/1.1
Authorization: Bearer <token>
```

Позволяет разблокировать звонок.

В ответ возвращается ответ с информацией о разблокированном звонке:

```json
{"retrieved": {...}}
```


## Информация о звонке

```http
GET /calls/123 HTTP/1.1
Authorization: Bearer <token>
```

Позволяет получить информацию о звонке.

В ответ возвращается ответ с информацией о звонке:

```json
{"callInfo": {...}}
```

Если такого звонка не было, он уже отвечен или завершен, то возвращается ошибка 404.


## Список голосовых сообщений пользователя (голосовая почта)

```http
GET /voicemails HTTP/1.1
Authorization: Bearer <token>
```

Возвращает список голосовых сообщений пользователя, упорядоченных по идентификатору:

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{
    "voiceMails": [
        {
            "from": "3095",
            "fromName": "Dmitry Sedykh",
            "to": "3095",
            "ownerType": "user",
            "id": "82",
            "received": 1502213652,
            "duration": 29,
            "read": true,
            "note": "text note"
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

Поддерживаемые поля: `from`, `fromName`, `callerName`, `to`, `ownerType`, `id`, `received` (_timestamp_), `duration` (_number_), `read` (_bool_), `note`. Пустые поля могут быть опущены.

## Файл с голосовым сообщением

```http
GET /voicemails/82 HTTP/1.1
Authorization: Bearer <token>
```

Идентификатор голосового сообщения задается в URL запроса. В ответ возвращается содержимое файла с голосовым сообщением:

```http
HTTP/1.1 200 OK
Content-Disposition: attachment; filename="u00043884851147406145/m0020.wav"
Content-Type: audio/wave

<data>
```

## Удаление голосового сообщения

```http
DELETE /voicemails/82 HTTP/1.1
Authorization: Bearer <token>
```

Идентификатор голосового сообщения задается в URL запроса.

## Заметки и статус голосового сообщения

```http
PATCH /voicemails/82 HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json; charset=utf-8

{"note":"text note"}
```

Идентификатор голосового сообщения задается в URL запроса.

Задает метку о прочтении и/или комментарий к голосовому сообщению:

```json
{
  "note": "text note",
  "read": true
}
```

Параметры:

- `note` - текст заметки
- `read` - флаг прочтения

Параметры являются не обязательными, но хотя бы один из них должен быть указан. Если параметр не указан, то значение по умолчанию не используется и данное действие просто не выполняется.

В ответ возвращает список использованных параметров:

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{
    "vm": {
        "note": "text note"
    }
}
```

## Серверная информация о конференция

```http
GET /conferences/info HTTP/1.1
Authorization: Bearer <token>
```

Возвращает настройки сервера конференций:

```json
{
    "conferences": {
        "ext": "",
        "did": "",
        "subject": "Conference Call: %Date% @ %Time% - %Subject%",
        "body": "Please join my conference call\n\nSubject: %Subject%\n\nDate: %Date%\nTime: %Time% %Timezone%\nDuration: %Duration%\n\nAt the scheduled date and time please call - %DID%\nInternal participants please call - %Extension%-%ID%\n\nIf prompted, enter the following Conference ID: %ID%, followed by # key.",
        "meeting": "View the MXmeeting web conference session at: http://\u003cinsert your domain name here\u003e/join?id=%ID%"
    }
}
```

## Список конференций

```http
GET /conferences HTTP/1.1
Authorization: Bearer <token>
```

Возвращает список зарегистрированных конференций:

```json
{
    "conferences": [
        {
            "Id": "",
            "ownerId": 0,
            "name": "",
            "accessId": 0,
            "type": "",
            "startDate": 0,
            "duration": 0,
            "waitForOwner": false,
            "delOnOwnerLeave": false,
            "ws": "",
            "wsType": ""
        }
    ]
}
```

## Удаление конференции

```http
DELETE /conferences/<id> HTTP/1.1
Authorization: Bearer <token>
```

## Создание конференции

```http
POST /conferences HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json; charset=utf-8

{
  "Id": "",
  "ownerId": 43892779647572507,
  "name": "Test Conf",
  "accessId": 47281964,
  "type": "Once",
  "startDate": 1516021200,
  "duration": 30,
  "waitForOwner": false,
  "delOnOwnerLeave": false,
  "ws": "None",
  "wsType": "Undefined"
}
```

## Изменение конференции

```http
PUT /conferences/<id> HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json; charset=utf-8

{
  "Id": "",
  "ownerId": 43892779647572507,
  "name": "Test Conf",
  "accessId": 47281964,
  "type": "Once",
  "startDate": 1516021200,
  "duration": 30,
  "waitForOwner": false,
  "delOnOwnerLeave": false,
  "ws": "None",
  "wsType": "Undefined"
}
```

## Присоединение к конференции

```http
POST /conferences/<id> HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json; charset=utf-8

{
    "accessId": 47281964,
}
```

Пока возвращается только ошибка. В случае удачного выполнения команды ответ пустой. В дальнейшем будет расширено.


## Создание конференции из звонка

```http
POST /calls/<id>/conference HTTP/1.1
Authorization: Bearer <token>
Content-Type: application/json; charset=utf-8

{
    "ownerCallId": 43,
}
```

Пока возвращается только ошибка. В случае удачного выполнения команды ответ пустой. В дальнейшем будет расширено.

## Регистрация токена устройства

```http
PUT /tokens/apn/com.connector73.vialer.voip/7C179108B7BF759DED2D9CBED7969DE6623D34E200E46387E7D713917E0F3EB8 HTTP/1.1
Authorization: Bearer <token>
```

В запроса передаются тип токена (`apn` или `fcm`), идентификатор приложения или темы для уведомления, а так же сам токен.

## Удаление токена устройства

```http
DELETE /tokens/apn/com.connector73.vialer.voip/7C179108B7BF759DED2D9CBED7969DE6623D34E200E46387E7D713917E0F3EB8 HTTP/1.1
Authorization: Bearer <token>
```

В запроса передаются тип токена (`apn` или `fcm`), идентификатор приложения или темы для уведомления, а так же сам токен.

## Файл конфигурации

- `provisioning` - задает адрес для авторизации пользователя и получения информации о настройках сервера MX. По умолчанию используется адрес <https://config.connector73.net/config>, поэтому задавать данное значение имеет смысл только в том случае, если вы хотите его переопределить.
- `apps` - задает список идентификаторов приложения `client-id` и секретной строки `secret` для авторизации приложений *OAuth2*. Раздел является обязательным и не может быть пустым. Подробнее об используемой авторизации приложений смотри [Resource Owner Password Credentials Grant](https://tools.ietf.org/html/rfc6749#section-4.3).
- `dbName` - задает путь и имя файла для хранения авторизационной информации пользователей и токенов устройств. Если не указано, то используется имя `mxproxy.db`.
- `logName` - задает полный путь для доступа к файлу с логом. Если задан, то лог будет доступен по запросу `GET /debug/log`.
- `voip` раздел используется для настройки _Voice over IP Push_:
    - `apnTTL` - время жизни пуш-клиентов для APNS, после которого они пересоздаются (для MS Azure); По умолчанию 10 минтут;
    - `apn` - список имен файлов с сертификатами для _Apple VoIP Push_ и паролей для их открытия;
    - `fcm` - список идентификаторов приложений и ключей для отправки уведомлений через _Google Firebase Cloud Messages_.
- `jwt` задает настройки для токенов авторизации:
    - `tokenTTL` - задает время валидности токена авторизации. По умолчанию - один час.
    - `signKeyTTL` - задает время жизни ключа для подписи токена, после которого ключ автоматически меняется. По умолчанию - 6 часов.
- `adminWeb` - адрес и порт административного сервера. По умолчанию, `http://localhost:8043`.

Пример конфигурационного файла:

```toml
[apps]
  client1 = "client-secret"
[voip]
  apnTTL = "3m50s"
[voip.apn]
  "certificate.p12" = "password"
[voip.fcm]
  "app" = "AAAA0bHpCVQ:APA9...p7Yge"
```

## Административный веб

Административный веб запускается по адресу `http://localhost:8043`. Адрес можно переопределить в конфигурационном файле. Если задать пустую строку, то сервер не запускается.

На нем доступны следующие данные:

- `GET /apps` - возвращает список идентификаторов зарегистрированных приложений
- `GET /connections` - возвращает список активных соединений с серверами МХ
- `GET /tokens` - возвращает список зарегистрированных токенов устройств
- `GET /users` - возвращает список зарегистрированных пользователей
- `POST /users` - удаляет пользователя и разрегистрирует его токены; логин пользователя передается в виде значения поля формы `login`

```shell
curl localhost:8043/users
{
    "users": {
        "dmitrys@xyzrd.com": {
            "host": "10.0.0.1:7778",
            "login": "dmitrys",
            "password": "xxxxx"
        }
    }
}
```

```shell
curl localhost:8043/users -d login=dmitrys@xyzrd.com
{
    "userLogout": "dmitrys@xyzrd.com"
}
```