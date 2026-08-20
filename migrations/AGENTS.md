# Правила миграций

- Миграции применяются до запуска trader и не обращаются к брокеру.
- Предпочитай forward-only изменения; destructive migration требует явного плана backup/restore и ручного review.
- Критичные инварианты заявок, executions и активных циклов закрепляй constraints/indexes.
- Не помещай credentials, account IDs и реальные trading data в fixtures.
- Проверяй migrations up, rollback транзакции и конкурентные ограничения на реальном PostgreSQL в integration tests.
- Схема-ориентир находится в `docs/architecture/data-model.md`.
