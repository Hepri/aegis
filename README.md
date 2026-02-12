# Aegis — родительский контроль

Сервис родительского контроля: клиент на Windows управляет доступом к учётным записям по расписанию, получая конфигурацию с сервера.

## Сборка

```bash
# Сервер (работает на любой ОС)
go build -o aegis-server ./cmd/aegis-server

# Клиент (только Windows)
GOOS=windows GOARCH=amd64 go build -o aegis-client.exe ./cmd/aegis-client
```

## Запуск сервера

```bash
./aegis-server -port 8080 [-data aegis-data.json]
```

Веб-интерфейс: http://localhost:8080

## Установка клиента на Windows

```powershell
aegis-client.exe install --server-url=http://server:8080
```

Опционально: `--client-id=UUID` для указания своего ID.

Удаление:

```powershell
aegis-client.exe uninstall
```

## API

- `GET /api/config?client_id=XXX` — long-poll, возвращает конфиг при изменении
- `GET /api/clients` — список компьютеров
- `POST /api/clients` — добавить компьютер
- `GET /api/clients/{id}` — конфиг компьютера
- `POST /api/clients/{id}/users` — добавить пользователя
- `PUT /api/clients/{id}/users/{uid}/schedule` — расписание
- `POST /api/clients/{id}/temporary-access` — выдать N минут (`{"user_id":"...","duration":120}`)
- `POST /api/clients/{id}/block` — заблокировать компьютер (`{"duration":120}`)
