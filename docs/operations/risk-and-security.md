# Риск-контроль и безопасность

## 1. Fail-safe принцип

Если система не может доказать безопасность команды, команда не отправляется. Потеря доступности допустима; неопределённая торговая операция — нет.

## 2. Risk checks перед каждой командой

### Общие

- режим разрешает действие;
- `approved=true` и snapshot не просрочен;
- стратегия не paused/error;
- reconciliation завершён без расхождений;
- stream/market data не устарели сверх заданного порога;
- инструмент доступен для API-торговли;
- торговая сессия и статус допускают заявку;
- цена соответствует шагу и допустимому диапазону;
- количество положительно и соответствует lot;
- нет другой команды того же логического назначения.

### BUY

- стоимость заявки не выше `max_order_rub`;
- резерв стратегии не выше `max_reserve_rub`;
- общий резерв не выше `max_total_reserve_rub`;
- достаточно свободных денежных средств с комиссионным буфером;
- дневной оборот не превышен;
- `confirmMarginTrade=false`.

### SELL

- количество не выше свободной позиции брокера;
- при `ladder_only` количество не выше net ladder inventory;
- protected base остаётся на счёте;
- заявка относится к текущему cycle;
- нет открытого конкурирующего SELL.

## 3. Kill switch

Разделяются две операции:

### `pause_new_orders`

- запрещает PLACE и REPLACE;
- не снимает существующие заявки;
- разрешает read-only reconciliation;
- вступает в силу после следующего polling либо локальной команды.

### `cancel_open_orders`

- отдельная ручная команда;
- требует аутентификации и явного scope;
- сначала формирует список целевых заявок;
- отменяет только заявки, принадлежащие сервису;
- сохраняет каждую отмену в audit.

Массовая отмена по account без фильтра стратегии запрещена в MVP.

## 4. Секреты

- `TINVEST_TOKEN` — Docker Secret или файл `0400` вне Git.
- Google service account JSON — read-only mount и доступ только к нужной таблице.
- `account_id` — серверная конфигурация/secret, в таблице только alias.
- Секреты не передаются через command-line arguments.
- В логах применяется redaction для metadata `authorization` и чувствительных полей.
- Токен production ограничивается одним счётом, если это позволяет кабинет.

## 5. Разделение контуров

- Разные токены для sandbox и prod.
- Разные Compose env files/secrets.
- В образе нет prod-конфигурации.
- `live` требует одновременно:
  - `APP_ENV=production`;
  - `TRADING_MODE=live`;
  - опубликованный `mode=live`;
  - `approved=true`;
  - непросроченный snapshot;
  - локальный файл-флаг/операторское разрешение.

Отсутствие любого условия означает запрет торговых методов.

## 6. Защита сервера

- SSH только по ключам.
- Firewall: входящие порты закрыты, кроме SSH/VPN.
- Operations API слушает `127.0.0.1` либо VPN interface.
- Контейнер запускается non-root, root filesystem read-only, где возможно.
- PostgreSQL не публикует порт наружу.
- Docker socket не монтируется в trader.
- Автоматические security updates ОС согласуются с окном обслуживания.
- NTP обязателен.

## 7. Supply chain

- Версии Go modules фиксируются.
- `go mod verify` в CI.
- SAST, dependency и image scan.
- Минимальный multi-stage image на distroless/alpine с проверкой совместимости CA.
- SBOM и digest образа сохраняются для live-релиза.
- Смена T‑Invest protobuf/SDK проходит regression suite.

## 8. Аудит

Записываются:

- применение/отклонение config snapshot;
- входное событие, приведшее к решению;
- результат Strategy Engine;
- решение Risk Engine с reason codes;
- command intent;
- ответ/ошибка брокера и tracking ID;
- все переходы state machine;
- административные pause/resume/cancel;
- reconciliation mismatches.

Audit append-only на уровне приложения. Retention — не менее срока проекта плюс период, необходимый для разбора спорных операций.

## 9. Алерты

Критичные:

- prod process перешёл в live;
- заявка `UNKNOWN`;
- reconciliation mismatch;
- продажа приблизилась к protected base;
- токен невалиден;
- БД недоступна;
- несколько экземпляров пытаются стать leader;
- snapshot mutation при том же revision;
- дневной лимит достигнут.

Предупреждения:

- stream reconnect;
- stale market data;
- заявка отклонена;
- Google Sheets недоступна;
- config приближается к `valid_until`.
