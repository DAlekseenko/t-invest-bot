# Интеграция с T‑Invest API

## 1. Протокол и контуры

Основной вариант — gRPC.

| Контур | Endpoint | Токен |
|---|---|---|
| Sandbox | `sandbox-invest-public-api.tbank.ru:443` | Sandbox token |
| Production | `invest-public-api.tbank.ru:443` | Full-access token выбранного счёта |

Endpoint, token и реальный `account_id` задаются серверной конфигурацией. Google Sheets содержит только `account_alias`.

## 2. Клиентская библиотека

Указанный в документации Go SDK находится в archived-репозитории. Интеграция должна быть изолирована адаптером.

Предпочтительный порядок:

1. Зафиксировать актуальную версию официальных protobuf-контрактов.
2. Сгенерировать Go gRPC client в отдельный package.
3. Реализовать `internal/tinvest` поверх generated client.
4. Не передавать protobuf-типы в domain packages.

Для ускорения spike допустимо временно использовать `github.com/russianinvestments/invest-api-go-sdk`, закрепив commit/version, но решение о production-зависимости фиксируется отдельно.

## 3. Используемые сервисы

### UsersService

- Получение доступных счетов.
- Проверка нужного account ID и режима доступа.

### InstrumentsService

- `GetInstrumentBy`/`FindInstrument`.
- Получение `instrument_uid`, `figi`, `class_code`, lot, currency, `min_price_increment`.
- Проверка `api_trade_available_flag`.

### MarketDataService / MarketDataStreamService

- Торговый статус.
- Last price и/или стакан для контроля цены и свежести данных.
- Stream используется для наблюдения, но лимитные BUY-заявки могут находиться у брокера без клиентского триггера.

### OrdersService

- `PostOrderAsync` или `PostOrder`.
- `GetOrders`.
- `GetOrderState`.
- `ReplaceOrder`.
- `CancelOrder`.
- `GetMaxLots` и `GetOrderPrice` как дополнительные preflight-проверки.

### OrdersStreamService

- Состояния заявок.
- Исполнения торговых поручений.
- Ping monitoring.

### OperationsService / OperationsStreamService

- Позиции и операции для восстановления пропущенных данных.

## 4. Параметры заявки MVP

| Поле | Значение |
|---|---|
| `instrumentId` | `instrument_uid` |
| `quantity` | количество лотов |
| `direction` | BUY или SELL |
| `orderType` | `ORDER_TYPE_LIMIT` |
| `timeInForce` | `TIME_IN_FORCE_DAY` |
| `orderId` | стабильный UUID/idempotency key |
| `priceType` | `PRICE_TYPE_CURRENCY` для акций/фондов |
| `confirmMarginTrade` | `false` |

Перед отправкой цена округляется к `min_price_increment` по явному правилу:

- BUY — не выше рассчитанной максимальной цены;
- SELL — не ниже рассчитанной минимальной цены.

## 5. Retry policy

### Можно повторять автоматически

- Read-only unary-запросы при transient gRPC codes.
- Reconnect stream с exponential backoff и jitter.
- Идемпотентную торговую команду только с тем же request ID и после проверки её состояния.

### Нельзя безусловно повторять

- `PostOrder` после timeout/connection reset.
- `ReplaceOrder`, если неизвестно, применена ли замена.
- `CancelOrder`, если локально не определена целевая заявка.

Пример backoff для stream: 1, 10, 60 секунд с jitter и верхним пределом. После восстановления всегда выполняется unary reconciliation.

## 6. Rate limiting

- Общий client-side limiter ниже документированного ограничения.
- Отдельные bucket для Instruments, MarketData, Orders и Sandbox.
- Реакция на `ResourceExhausted/429` учитывает server metadata, если она доступна.
- Polling не дублирует данные, уже поступающие через stream.

Для трёх инструментов рабочая нагрузка существенно ниже лимитов, но limiter обязателен для защиты от циклической ошибки.

## 7. Sandbox caveats

Sandbox нужен для проверки контрактов, автомата и восстановления, но не является моделью реальной ликвидности:

- заявки не влияют на стакан;
- лимитная заявка может исполниться целиком при наличии встречного объёма только на один лот;
- комиссии и параметры портфеля моделируются упрощённо;
- неисполненные заявки снимаются после торговой сессии.

Поэтому обязательны два этапа после sandbox: prod `dry_run`, затем ограниченный `live`.

## 8. Ошибки и классификация

| Класс | Пример | Действие |
|---|---|---|
| Authentication | invalid/expired token | `PAUSED`, alert, без retry-loop |
| Authorization | account mismatch | `ERROR`, требует настройки |
| Validation | invalid price increment | отклонить команду, исправить конфигурацию |
| Trading restriction | instrument unavailable | пауза стратегии |
| Rate limit | 429/ResourceExhausted | backoff |
| Transient | 503/504/unavailable | retry read; command → `UNKNOWN` |
| Business reject | insufficient funds/position | сохранить reject, reconciliation |

Сохраняются gRPC code, broker message и `x-tracking-id`, если доступен. Токен и чувствительные metadata не сохраняются.

## 9. Официальные источники

- [Начало работы](https://developer.tbank.ru/invest/intro/intro)
- [OrdersService](https://developer.tbank.ru/invest/api/orders-service)
- [PostOrderAsync](https://developer.tbank.ru/invest/api/orders-service-post-order-async)
- [Stream-соединения](https://developer.tbank.ru/invest/intro/developer/stream)
- [Лимиты](https://developer.tbank.ru/invest/intro/intro/limits)
- [Песочница](https://developer.tbank.ru/invest/intro/developer/sandbox)
- [Токены](https://developer.tbank.ru/invest/intro/intro/token)
- [Go SDK](https://github.com/RussianInvestments/invest-api-go-sdk)
