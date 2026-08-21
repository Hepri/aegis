# Deploy Aegis Server

## Использование

```bash
# Первичный деплой (создание каталога, установка systemd, запуск + OTA-пакет клиента)
./deploy.sh initial

# Редеплой (сервер + новый Windows-клиент для удалённого обновления)
./deploy.sh redeploy
# или просто
./deploy.sh

# Только обновить клиент на сервере (без рестарта сервера)
./deploy.sh client-only
```

Клиенты на Windows сами скачают новую версию при следующем long-poll конфига (поле `update` в `/api/config`).

Версию клиента можно задать явно:

```bash
CLIENT_VERSION=20260821.1 ./deploy.sh redeploy
```

## Переменные окружения

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| DEPLOY_IP | 192.168.0.234 | IP сервера |
| DEPLOY_USER | aegis | Пользователь SSH |
| DEPLOY_PATH | /opt/aegis | Путь на сервере |
| CLIENT_VERSION | UTC timestamp | Версия Windows-клиента для OTA |

На сервере появляются файлы:

- `$DEPLOY_PATH/updates/aegis-client.exe`
- `$DEPLOY_PATH/updates/client.json`

## Требования

1. **SSH-ключ** — `ssh-copy-id aegis@192.168.0.234`
2. **Парольный sudo** — один раз на сервере выполнить:
   ```bash
   scp deploy/sudoers.aegis aegis@192.168.0.234:/tmp/
   ssh aegis@192.168.0.234
   sudo cp /tmp/sudoers.aegis /etc/sudoers.d/aegis-deploy
   sudo chmod 440 /etc/sudoers.d/aegis-deploy
   ```

## Ошибка "No route to host"

Не удаётся достучаться до сервера. Проверьте:

- Вы в той же сети, что и 192.168.0.234
- Сервер запущен
- IP правильный (если другой — `DEPLOY_IP=x.x.x.x ./deploy.sh initial`)
