# Правила T‑Invest adapter

- Изолируй gRPC/generated SDK внутри adapter; наружу возвращай доменные типы.
- Sandbox и production используют один контракт, но разные endpoints, tokens и configuration.
- Торговые запросы принимают только stable request ID, подготовленный Execution Engine.
- Read-only запросы можно повторять при transient errors; неопределённые trading commands — только по правилам reconciliation.
- После stream reconnect выполняй unary reconciliation.
- Сохраняй gRPC code и tracking ID, но не authorization metadata, токены или полный payload с секретами.
- Проверяй lot, price increment, trading status и API availability по broker metadata.
- Детали интеграции находятся в `docs/integrations/tinvest.md`.
