# Развёртывание

## 1. Целевая среда

- Один выделенный Linux-сервер.
- Docker Engine и Docker Compose v2.
- Исходящие соединения к Google API и T‑Invest API.
- Входящий доступ только SSH/VPN.
- Синхронизация времени NTP.

## 2. Compose services

### Обязательные

- `trader` — Go-приложение;
- `postgres` — состояние и аудит;
- `migrate` — one-shot применение миграций.

### Наблюдаемость

- `prometheus`;
- `grafana`;
- `alertmanager` — после настройки канала уведомлений.

## 3. Volumes

- `postgres-data` — постоянные данные PostgreSQL;
- `prometheus-data` — метрики;
- `grafana-data` — dashboards;
- read-only bind mount Google credentials;
- Docker Secrets для broker token и DB password.

Trader не получает доступ к Docker socket и директории БД.

## 4. Переменные окружения

Для локального Compose значения находятся в `deploy/.env`. Файл создаётся командой `make infra-init` из версионируемого шаблона `deploy/.env.example`, получает случайный `DATABASE_PASSWORD`, исключён из Git и явно передаётся Compose через `--env-file`. Production credentials передаются через secret files/manager, не через committed `.env`.

Несекретные:

```dotenv
APP_ENV=development
TRADING_MODE=disabled
LOG_LEVEL=info
HTTP_ADDR=127.0.0.1:8080
METRICS_ADDR=127.0.0.1:9090
DATABASE_HOST=postgres
DATABASE_NAME=ladder_trader
DATABASE_USER=ladder_trader
GOOGLE_SPREADSHEET_ID=1mIV7iEtcosyktZi6HyO3xny1Q1F88Pcf-jLwj2esuZ8
GOOGLE_CONTROL_RANGE=BOT_CONTROL!A:B
GOOGLE_CONFIG_RANGE=BOT_CONFIG!A:S
CONFIG_POLL_INTERVAL=30s
RECONCILE_INTERVAL=60s
TINVEST_ENDPOINT=sandbox-invest-public-api.tbank.ru:443
TINVEST_ACCOUNT_ALIAS=main
```

Локальный пароль генерируется только в игнорируемом `deploy/.env`:

```dotenv
DATABASE_PASSWORD=<local generated value>
```

Секретные значения не помещаются в `.env` внутри репозитория:

```text
TINVEST_TOKEN_FILE=/run/secrets/tinvest_token
TINVEST_ACCOUNT_ID_FILE=/run/secrets/tinvest_account_id
DATABASE_PASSWORD_FILE=/run/secrets/postgres_password  # production alternative
GOOGLE_CREDENTIALS_FILE=/run/secrets/google_credentials.json
```

## 5. Startup sequence

1. PostgreSQL healthcheck.
2. Применение migrations.
3. Запуск trader в `disabled`.
4. Проверка Google credentials и чтение control/config.
5. Проверка токена и account mapping read-only методом.
6. Загрузка instrument metadata.
7. Reconciliation.
8. Захват leader lock.
9. Readiness становится успешной.
10. Режим может повыситься только согласно многослойной проверке.

## 6. Healthchecks

### Liveness

- event loop не заблокирован;
- HTTP endpoint отвечает.

### Readiness

- PostgreSQL доступна;
- migrations актуальны;
- leader lock получен;
- config snapshot валиден;
- reconciliation не требуется;
- для trading readiness доступен broker API.

Потеря readiness не должна перезапускать контейнер бесконечно и не должна автоматически снимать заявки.

## 7. Релиз

1. CI создаёт versioned image и SBOM.
2. Image публикуется по digest.
3. На сервере выполняется backup БД.
4. `pause_new_orders=true` или локальная пауза.
5. Проверяется отсутствие `UNKNOWN` commands.
6. `docker compose pull`.
7. `docker compose run --rm migrate`.
8. `docker compose up -d`.
9. Проверка health/readiness.
10. Ручной reconciliation.
11. Возврат сначала в `dry_run`; live — отдельным действием.

## 8. Backup и restore

- Ежедневный `pg_dump` с шифрованием.
- Ротация: 7 ежедневных, 4 еженедельных, 3 ежемесячных копии.
- Копия хранится вне сервера.
- Restore test до запуска live и затем ежеквартально.
- Google Sheets не считается backup состояния исполнения.

## 9. Rollback

- Остановить новые команды.
- Не выполнять `down -v`.
- Зафиксировать broker state.
- Откатить image на предыдущий digest.
- Forward-only migration предпочтительнее down migration.
- Запустить reconciliation до возобновления.

## 10. Runbook аварийной остановки

1. Установить локальную паузу.
2. Проверить открытые заявки через брокерское приложение/API.
3. Сопоставить с `/v1/status` и БД.
4. При необходимости вызвать scoped `cancel-open-orders` только для заявок робота.
5. Не удалять контейнеры и volume до сохранения диагностики.
6. Сохранить логи, tracking IDs и reconciliation report.
