# Правила Execution Engine

- До внешнего вызова сохраняй command intent и стабильный idempotency key в PostgreSQL.
- Поддерживаются только LIMIT DAY orders; margin confirmation всегда выключен.
- Timeout или потеря ответа переводят команду в `UNKNOWN`; не создавай новую логическую команду до reconciliation.
- Повтор допустим только с тем же request ID и после проверки состояния у брокера.
- Сетевой вызов не выполняется внутри долгой SQL-транзакции.
- Повторные и устаревшие broker events обрабатываются идемпотентно.
- После SELL fill цикл закрывается только после подтверждённого состояния остальных заявок и отсутствия `UNKNOWN` commands.
- Изменения покрывай тестами partial fill и рестарта между фиксацией intent и ответом брокера.
- Модель переходов находится в `docs/architecture/trading-state-machine.md`.
