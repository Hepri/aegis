# Aegis — родительский контроль

Сервис родительского контроля: клиент на Windows управляет доступом к учётным записям по расписанию, получая конфигурацию с сервера. Клиент также собирает логины/приложения и умеет обновляться удалённо (OTA).

## Сборка

```bash
# Сервер (работает на любой ОС)
go build -o aegis-server ./cmd/aegis-server

# Клиент (только Windows), с версией для OTA
GOOS=windows GOARCH=amd64 go build -ldflags "-X main.Version=$(date -u +%Y%m%d%H%M%S)" -o aegis-client.exe ./cmd/aegis-client
```

## Запуск сервера

```bash
./aegis-server -port 8080 [-data aegis-data.json] [-updates ./updates]
```

Веб-интерфейс: http://localhost:8080

Каталог `updates/` рядом с data-файлом (или `-updates`) должен содержать `client.json` + `aegis-client.exe` для OTA.

## Установка клиента на Windows

Первый раз — вручную:

```powershell
aegis-client.exe install --server-url=http://server:8080 --client-id=UUID
# или --client-name="Home PC"
```

Дальнейшие обновления — через `./deploy/deploy.sh` (сервер публикует новый exe, клиент сам подтягивает).

Удаление:

```powershell
aegis-client.exe uninstall
```

## API

- `GET /api/config?client_id=XXX` — long-poll, возвращает конфиг при изменении (+ `update` при наличии OTA)
- `POST /api/clients/{id}/events` — батч событий активности с клиента
- `GET /api/clients/{id}/activity?date=YYYY-MM-DD` — агрегат за день (сессии, приложения, таймлайн)
- `GET /api/updates/client` — манифест OTA
- `GET /api/updates/aegis-client.exe` — бинарник клиента
- `GET /api/clients` — список компьютеров (`online`, `last_seen`)
- `POST /api/clients` — добавить компьютер
- `GET /api/clients/{id}` — конфиг компьютера
- `POST /api/clients/{id}/users` — добавить пользователя
- `PUT /api/clients/{id}/users/{uid}/schedule` — расписание
- `POST /api/clients/{id}/temporary-access` — выдать N минут (`{"user_id":"...","duration":120}`)
- `POST /api/clients/{id}/block` — заблокировать компьютер (`{"duration":120}`)
