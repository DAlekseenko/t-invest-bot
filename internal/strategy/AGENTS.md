# Правила Strategy Engine

- Модуль детерминирован и не выполняет I/O: одинаковый snapshot, cycle и broker state дают одинаковый desired state.
- Расчёты цен и денег выполняются fixed-point; округление к price increment задаётся явно.
- SELL рассчитывается по фактическим executions, а не по исходному размеру заявок.
- При `ladder_only` продажа не превышает net ladder inventory и не уменьшает protected base.
- На цикл существует не более одного логического SELL.
- Новые переходы покрывай unit-тестом, повторной доставкой события, out-of-order событием и partial fill, если они применимы.
- Состояния и переходы определены в `docs/architecture/trading-state-machine.md`.
