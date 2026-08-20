# Roadmap реализации

## Этап 0. Зафиксировать семантику — 1–2 дня

- Утвердить [открытые вопросы](open-questions.md).
- Создать `BOT_CONTROL` и `BOT_CONFIG`.
- Экспортировать golden fixture текущей таблицы.
- Зафиксировать formula/rounding rules.
- Определить protected base для каждого инструмента.

Результат: подписанный контракт конфигурации и тестовые примеры.

## Этап 1. Каркас проекта — 2–3 дня

- Инициализировать Go module.
- Поднять Compose: trader, postgres, migrate.
- Добавить config loader, structured logging, health endpoints.
- Создать migrations и repositories.
- Настроить CI quality gates.

Результат: приложение стартует в `disabled`, мигрирует БД и проходит CI.

## Этап 2. Read-only интеграции — 2–3 дня

- Google Sheets adapter.
- T‑Invest auth/accounts.
- Instruments metadata.
- Positions/orders read model.
- Market/order streams с reconnect.

Результат: сервис показывает фактические позиции и заявки, но не торгует.

## Этап 3. Strategy и Risk Engines — 3–4 дня

- Fixed-point arithmetic.
- Расчёт шести уровней.
- Desired state.
- Политика protected base.
- Golden и property-based tests.

Результат: dry-run объясняет каждую потенциальную заявку.

## Этап 4. Execution и reconciliation — 4–6 дней

- Command journal.
- Stable idempotency keys.
- Place/replace/cancel.
- Partial fills.
- State machine.
- Reconciliation и unknown command recovery.

Результат: полный торговый цикл работает на fake broker.

## Этап 5. Sandbox — 3–5 дней

- Sandbox account и funding.
- SBER/TRNFP/BTBR fixtures.
- Restart/failure сценарии.
- Session-end DAY order recovery.
- Метрики и алерты.

Результат: все E2E сценарии пройдены, нет дублей.

## Этап 6. Prod shadow — минимум 5 торговых дней

- Production read-only/full token без вызова торговых методов.
- Сверка рекомендаций с таблицей.
- Наблюдение stream и расписания торгов.
- Daily report.

Результат: подтверждена стабильность на реальных данных.

## Этап 7. Limited live — 5–10 торговых дней

- SBER.
- Первый уровень.
- Малый лимит заявки.
- `auto_restart=false`.
- Ручное наблюдение полного цикла.

Результат: подтверждён реальный цикл без mismatch.

## Этап 8. Постепенное расширение

1. Все уровни SBER.
2. TRNFP.
3. BTBR.
4. Решение по автоматическому новому циклу.
5. Решение по status dashboard/write-back.

## Milestones

| Milestone | Критерий |
|---|---|
| M1: Read-only | Видим таблицу, инструменты, позиции и заявки |
| M2: Deterministic | Golden tests совпадают |
| M3: Safe execution | Fake broker failure suite пройдена |
| M4: Sandbox ready | E2E и restart recovery пройдены |
| M5: Live candidate | 5 дней shadow без критических расхождений |
| M6: MVP live | Один реальный цикл SBER успешно закрыт |

## Приоритет первого спринта

1. Go skeleton и dependency boundaries.
2. PostgreSQL migrations.
3. Google Sheets parser/validator.
4. T‑Invest read-only adapter.
5. Golden fixtures SBER/TRNFP/BTBR.
6. Dry-run report.
