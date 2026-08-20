# Правила процесса trader

- Процесс всегда стартует в `disabled`, независимо от предыдущего режима.
- Startup order: migrations, config validation, read-only broker checks, metadata, reconciliation, leader lock, readiness.
- До успешного reconciliation и leader lock торговые методы недоступны.
- Readiness failure не инициирует отмену заявок и не должен создавать бесконечный restart loop.
- Сигналы остановки отменяют общий context и дожидаются всех принадлежащих процессу goroutine.
