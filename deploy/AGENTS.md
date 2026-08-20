# Правила развёртывания

- Образы и compose-конфигурация по умолчанию запускают trader в `disabled`.
- Production secrets передаются через secret files/manager, не через Git, image, CLI arguments или committed `.env`.
- Sandbox и production используют разные tokens, endpoints и env/secrets.
- Trader работает non-root, без Docker socket и прямого доступа к PostgreSQL volume.
- Operations API не публикуется в интернет; PostgreSQL не открывает внешний порт.
- Release в live требует backup, отсутствия `UNKNOWN` commands, ручного reconciliation и отдельного повышения режима.
- Rollback не удаляет volumes и также завершается reconciliation.
- Runbook находится в `docs/operations/deployment.md`.
