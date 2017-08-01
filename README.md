# MX CSTA Proxy

## Параметры запуска

```ssh
Usage of ./mxproxy:
  -config filename
      config filename (default "mxproxy.json")
  -csta
      CSTA output
  -db filename
      tokens DB filename (default "mxproxy.db")
  -debug
      debug output
  -host name
      main server name (default "mxproxy.connector73.net")
  -logflag int
      log flags (default 64)
```

Для запуска локальной копии сервера используйте ключ `-host localhost:8080`: в этом случае сервис не будет пытаться использовать HTTPS и получать сертификат Let's Encrypt.

Флаг `-debug` выводит в лог некоторую дополнительную отладочную информацию (ее не очень много).

Флаг `-csta` включает вывод всех команд и сообщений CSTA в стандартный вывод консоли (в отличие от вывода лога в `stderr`), который, при желании, можно перенаправить в файл.


## Конфигурационный файл

```json
{
  "mx": {
    "test": {
      "addr": "127.0.0.1:7778",
      "login": "server",
      "password": "password"
    }
  },
  "apns": {
    "push-cert.p12": "password"
  },
  "gfcm": {
    "app-name": "GOOGLE-APP-KEY"
  }
}

```

- `mx` содержит список имен MX серверов и параметры для подключения
- `apns` - список сертификатов Apple и пароли для их чтения
- `gfcm` - список приложений Google и ключи для отправки им уведомлений


## Запросы

Все запросы требуют авторизации MX. Авторизационная информация передается в заголовке HTTP-запроса с использованием HTTP Basic формата.

Запросы имеют вид:

```url
/mx/<mx-name>/<command>
```

В случае успешного выполнения команды возвращается статус HTTP 2xx. Статусы 4xx и 5xx возвращаются в случае ошибок. В последнем случае, как правило, в теле ответа содержится описание ошибки в формате JSON:

```json
{
  "error": "not found"
}
```


### Адресная книга

```http
GET /mx/<mx-name>/addressbook
```

Возвращает адресную книгу MX:

```json
{
  "addressbook": {
    "43884850646482261": {
        "cellPhone": "+1-512-555-0136",
        "email": "mikef@xyzrd.com",
        "ext": "3055",
        "firstName": "Mike",
        "homePhone": "+1-202-555-0104",
        "lastName": "Flynn",
        "status": "LoggedOut"
    },
    "43884850647480796": {
        "ext": "3093",
        "firstName": "Test",
        "lastName": "Admin",
        "status": "LoggedOut"
    },
    ...
  }
}
```


### Список звонков

```http
GET /mx/<mx-name>/calllog
```

Возвращает список звонков пользователя:

```json
{
  "callog": [
    {
      "callType": 1,
      "callingPartyNo": "3044",
      "direction": "outgoing",
      "disconnect": "2017-07-26T20:32:51Z",
      "ext": "3099",
      "firstName": "Maxim",
      "gcid": "63022-00-0000D-175",
      "lastName": "Donchenko",
      "legType": 1,
      "originalCalledPartyNo": "3099",
      "recordId": 373,
      "selfLegType": 1
    },
    {
      "callType": 1,
      "callingPartyNo": "3044",
      "connect": "2017-07-26T20:35:20Z",
      "direction": "outgoing",
      "disconnect": "2017-07-26T20:35:33Z",
      "gcid": "63022-00-0000D-176",
      "legType": 1,
      "originalCalledPartyNo": "*86",
      "recordId": 374,
      "selfLegType": 1
    },
    {
      "callType": 1,
      "callingPartyNo": "3044",
      "connect": "2017-07-28T14:31:11Z",
      "direction": "outgoing",
      "disconnect": "2017-07-28T14:31:47Z",
      "ext": "3095",
      "firstName": "Dmitry",
      "gcid": "63022-00-0000D-1AB",
      "lastName": "Sedykh",
      "legType": 3,
      "originalCalledPartyNo": "3095",
      "recordId": 427,
      "selfLegType": 1
    },
    ...
  ]
}
```

Опционально можно указать в запросе ограничение по времени, начиная с которого будет отдаваться лог:

```http
GET /mx/<mx-name>/calllog?timestamp=2017-07-28T14:31:47Z
```

### Обратный звонок

```http
POST /mx/<mx-name>call
```

В качестве параметров в запросе можно передать JSON:

```json
{
  "ringDelay": 2,
  "vmDelay": 28,
  "from": "79031744445",
  "to": "79031744437"
}
```

Обязательными полями являются только `from` и `to`. `ringDelay` по умолчанию равен `1`, а `vmDelay` - 30:

```json
{
  "from": "79031744445",
  "to": "1099"
}
```

В ответ возвращается описание о звонке, полученное с сервера MX:

```json
{
  "callId": 32,
  "called": "1099",
  "deviceId": "3095"
}
```


### Регистрация токена устройства

```http
POST /mx/<ma-name>/token/<apns|gfcm>/<bundle-id>
```

В теле запроса передается сам токен:

```json
{
  "token": "7C179108B7BF759...34E200E46387E7D713917E0F3E09"
}
```

Поддерживаются только токены для Apple Push Notification и Google Firebase Cloud Messaging. Для Apple необходимо указать в конфигурации соответствующие сертификаты (bundleId читается непосредственно из него), а для Google - название приложения и ключ. Если этого не сделать, то такие `bundle-id` будут считаться не поддерживаемыми и будет возвращаться ошибка. В случае успешного сохранения токена возвращается пустой ответ.

Для Apple поддерживается опциональный параметр `sandbox` для указания, что данный токен именно для тестового устройства:

```http
POST /mx/test/token/apns/com.xyzrd.PushTest?sandbox
```

При добавлении токена в хранилище автоматически запускается мониторинг входящих звонков пользователя и, в случае такого звонка, ему автоматически на все зарегистрированные устройства отправляется уведомление через соответствующие службы Apple и Google.
