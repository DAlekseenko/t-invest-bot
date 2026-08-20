# Модель данных PostgreSQL

## 1. Общие правила

- Первичные ключи — UUID.
- Денежные значения хранятся fixed-point: `BIGINT` nano units либо `NUMERIC(28,9)`; один вариант выбирается до миграций и используется везде.
- Время — `TIMESTAMPTZ` в UTC.
- Broker IDs хранятся как строки без попытки интерпретации.
- Payload внешних ответов может храниться в `JSONB` после удаления чувствительных данных.
- Обновление доменного состояния и outbox выполняется одной транзакцией.

## 2. Таблицы

### `broker_accounts`

| Поле | Назначение |
|---|---|
| `id` | внутренний UUID |
| `alias` | `main` |
| `broker_account_id_ciphertext` | защищённый account ID либо ссылка на secret |
| `environment` | sandbox/prod |
| `enabled` | активность |
| `created_at`, `updated_at` | аудит |

### `instruments`

| Поле | Назначение |
|---|---|
| `instrument_uid` | ключ инструмента |
| `figi`, `ticker`, `class_code` | идентификаторы |
| `lot` | бумаг в лоте |
| `currency` | валюта расчётов |
| `min_price_increment_nano` | минимальный шаг |
| `api_trade_available` | доступность API |
| `metadata_at` | актуальность справочника |

### `config_snapshots`

| Поле | Назначение |
|---|---|
| `id` | snapshot UUID |
| `schema_version` | версия контракта |
| `revision` | опубликованная ревизия |
| `content_hash` | SHA-256 нормализованного содержимого |
| `mode` | режим из control plane |
| `approved` | подтверждение |
| `payload` | нормализованный JSONB |
| `published_at`, `loaded_at` | время |

Ограничение: `UNIQUE(revision)`, а изменение hash для существующей ревизии вызывает ошибку.

### `strategies`

- `id`;
- `external_strategy_id`;
- `account_id`;
- `instrument_uid`;
- `active_snapshot_id`;
- `enabled`;
- `sell_scope`;
- `protected_base_qty`;
- `max_reserve_nano`;
- timestamps.

Ограничение: `UNIQUE(account_id, external_strategy_id)`.

### `ladder_levels`

- `id`;
- `snapshot_id`;
- `strategy_id`;
- `level_no`;
- `step_budget_nano`;
- `buy_price_nano`;
- `buy_lots`;
- `sell_price_nano`;
- `preview_sell_qty`;
- corrections;
- timestamps.

Ограничение: `UNIQUE(snapshot_id, strategy_id, level_no)`.

### `strategy_cycles`

- `id`;
- `strategy_id`;
- `snapshot_id`;
- `cycle_no`;
- `state`;
- `started_at`, `closed_at`;
- `pause_reason`;
- `version` для optimistic locking.

Ограничения:

- `UNIQUE(strategy_id, cycle_no)`;
- partial unique index: не более одного незакрытого цикла на стратегию.

### `orders`

- `id`;
- `cycle_id`;
- `level_id`, nullable для агрегированного SELL;
- `side`;
- `command_revision`;
- `idempotency_key`;
- `broker_order_id`;
- `price_nano`;
- `requested_lots`;
- `executed_lots`;
- `status`;
- `submitted_at`, `updated_at`;
- `raw_status`.

Ограничения:

- `UNIQUE(idempotency_key)`;
- `UNIQUE(broker_order_id)` при ненулевом значении;
- логический unique key `(cycle_id, level_id, side, command_revision)`.

### `order_commands`

Командный журнал до внешнего вызова:

- `id`;
- `order_id`;
- `command_type` — place/replace/cancel;
- `idempotency_key`;
- `status` — persisted/submitting/acknowledged/unknown/failed;
- `attempt_no`;
- request/response metadata;
- timestamps.

### `executions`

- `id`;
- `order_id`;
- `broker_execution_id`;
- `quantity`;
- `price_nano`;
- `commission_nano`;
- `executed_at`;
- `raw_payload`.

Ограничение: `UNIQUE(broker_execution_id)`.

### `position_snapshots`

- `id`;
- `account_id`;
- `instrument_uid`;
- total/free/blocked quantity;
- average price при наличии;
- `source_at`, `loaded_at`.

### `reconciliation_runs`

- `id`;
- `account_id`;
- `reason`;
- `status`;
- `mismatch_count`;
- `details`;
- `started_at`, `finished_at`.

### `audit_events`

- `id`;
- `event_type`;
- `strategy_id`, `cycle_id`, `order_id`;
- `actor` — system/operator/config;
- `trace_id`;
- `payload`;
- `created_at`.

Таблица append-only для приложения.

### `outbox_events`

- `id`;
- `topic`;
- `payload`;
- `created_at`;
- `published_at`;
- `attempts`.

## 3. Ключевые инварианты

```text
executed_lots <= requested_lots
ladder_inventory >= 0
sell_quantity <= broker_free_quantity
sell_quantity <= ladder_inventory when sell_scope=ladder_only
reserved_amount <= strategy.max_reserve
one active logical SELL per cycle
one active cycle per strategy
```

Проверки дублируются на уровне domain, а критичные ограничения — в БД.

## 4. Транзакционные границы

В одной транзакции:

- обработка уникального broker event;
- изменение order/cycle state;
- запись audit event;
- запись outbox event.

Сетевой вызов брокера не выполняется внутри долгой SQL-транзакции. Перед ним фиксируется command intent, после него — результат отдельной транзакцией.

## 5. Блокировки

- Advisory lock на hash `(account_id, strategy_id)` перед reconcile/apply.
- `SELECT ... FOR UPDATE` для конкретного cycle/order.
- Optimistic version для административных изменений.
- Standby-процесс не торгует без лидерского lock.
