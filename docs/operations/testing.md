# Стратегия тестирования

## 1. Цели

- Доказать детерминированность расчётов.
- Исключить повторные заявки.
- Проверить восстановление после любого штатного отказа.
- Проверить, что Risk Engine запрещает опасные команды.
- Отделить корректность интеграции от реалистичности исполнения.

## 2. Пирамида тестов

### Unit

- fixed-point arithmetic и округление к price increment;
- парсинг Google Sheets;
- config validation;
- Strategy Engine;
- Risk Engine;
- state transitions;
- idempotency key generation;
- расчёт net ladder inventory;
- redaction секретов.

### Property-based / invariant tests

Для произвольных допустимых входов:

- BUY-уровни строго убывают;
- количество и цена неотрицательны;
- SELL не превышает разрешённую позицию;
- protected base не уменьшается;
- один event не применяется дважды;
- один cycle не имеет два активных логических SELL;
- сумма резервов не превышает лимит.

### Repository integration

На реальном PostgreSQL через testcontainers:

- migrations up;
- unique constraints;
- command intent transaction;
- outbox atomicity;
- advisory lock;
- конкурентная обработка одного события;
- recovery после rollback.

### Broker contract

Fake gRPC server должен моделировать:

- успешную заявку;
- reject;
- timeout до/после приёма команды;
- duplicate request ID;
- partial fill;
- out-of-order events;
- duplicate events;
- stream disconnect;
- missing ping;
- stale read model;
- 429/ResourceExhausted;
- unknown external order.

### Sandbox E2E

- создание sandbox account;
- пополнение;
- публикация тестового snapshot;
- BUY → fill → SELL → close;
- несколько BUY при gap-like движении;
- перезапуск контейнера с открытыми заявками;
- завершение торговой сессии и исчезновение DAY-заявок;
- reconnect stream и reconciliation.

Sandbox не используется для оценки slippage и глубины рынка.

## 3. Golden tests Google Sheets

Fixture хранится в Git независимо от живой таблицы.

Минимальный набор:

```yaml
sber:
  base_qty: 58
  base_avg_price: "284.00"
  level_1:
    budget: "6000.00"
    buy_price: "275.48"
    buy_lots: 21

trnfp:
  base_qty: 28
  base_avg_price: "1055.59"
  level_1:
    budget: "5000.00"
    buy_price: "1028.40"
    buy_lots: 4

btbr:
  base_qty: 142
  base_avg_price: "118.00"
  level_1:
    budget: "4000.00"
    buy_price: "114.46"
    buy_lots: 34
```

Отдельные тесты сравнивают все шесть BUY-уровней из [контракта Google Sheets](../integrations/google-sheets.md).

Если Go-расчёт отличается от preview:

- тест падает;
- изменение объясняется в ADR/changelog;
- silent tolerance запрещён;
- для цены допускается только заранее определённое округление к broker increment.

## 4. State machine scenarios

Обязательные сценарии:

1. Первый BUY полностью исполнен, SELL полностью исполнен.
2. Первый BUY исполнен, затем второй BUY, SELL заменён.
3. Два BUY исполнились до поступления stream event.
4. BUY и SELL исполняются частично.
5. Дубликат execution event.
6. События пришли не по порядку.
7. Рестарт до отправки команды.
8. Рестарт после отправки, но до сохранения ответа.
9. Timeout с заявкой, фактически созданной у брокера.
10. Timeout без созданной заявки.
11. Пользователь вручную отменил заявку.
12. Пользователь вручную создал неизвестную заявку по тому же инструменту.
13. Недостаточно денег.
14. Недостаточно свободной позиции.
15. Изменена уже опубликованная revision.
16. Включена пауза во время активного цикла.
17. Торговый день закончился, DAY-заявки сняты.

## 5. Dry-run / shadow

На prod market data не менее пяти торговых дней:

- рассчитывать команды без вызова OrdersService;
- сохранять виртуальные intents;
- сравнивать триггеры с фактическими ценами;
- измерять stale data и reconnect;
- ежедневно формировать отчёт «что было бы выставлено»;
- вручную сверять с Google Sheets.

## 6. Limited live

Порядок допуска:

1. Только SBER.
2. Только первый уровень.
3. Лимит заявки не выше согласованного малого бюджета.
4. `auto_restart=false`.
5. Наблюдение одного полного цикла.
6. Затем шесть уровней SBER.
7. Затем TRNFP.
8. Затем BTBR.

Расширение возможно только при нулевых необъяснимых mismatch и дублях.

## 7. CI quality gates

```text
go test ./...
go test -race ./...
go vet ./...
golangci-lint run
go mod verify
docker build
integration tests with PostgreSQL
secret scan
dependency/image scan
```

Тесты, способные обращаться к prod OrdersService, в CI отсутствуют архитектурно.
