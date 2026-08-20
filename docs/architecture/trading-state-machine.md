# Торговый автомат

## 1. Агрегаты

### Strategy

Постоянная конфигурация инструмента и правил.

### Cycle

Один проход лесенки от публикации BUY-уровней до исполнения выхода или ручной остановки.

### Level

Один BUY-уровень и его фактические исполнения.

### Order

Локальное представление торгового поручения брокеру.

### Execution

Уникальная исполненная сделка или часть заявки.

## 2. Состояния цикла

| Состояние | Значение |
|---|---|
| `DRAFT` | Snapshot загружен, но не активирован |
| `PUBLISHED` | Конфигурация принята |
| `RECONCILING` | Идёт сверка с брокером |
| `BUY_ORDERS_ACTIVE` | Активны BUY-уровни, исполнения ещё нет |
| `POSITION_ACCUMULATING` | Есть BUY-исполнения и активный выход |
| `EXIT_PARTIALLY_FILLED` | SELL исполнен частично |
| `CLOSING` | Завершаются отмены оставшихся заявок |
| `CLOSED` | Цикл завершён |
| `PAUSED` | Новые команды запрещены оператором или control plane |
| `RECONCILIATION_REQUIRED` | Есть необъяснимое расхождение |
| `ERROR` | Неустранимая ошибка конфигурации/домена |

## 3. Состояния заявки

```text
PLANNED
  → PERSISTED
  → SUBMITTING
  → NEW | PARTIALLY_FILLED | FILLED
  → CANCELLING → CANCELLED
  → REPLACING → REPLACED
  → REJECTED
  → UNKNOWN
```

`UNKNOWN` означает, что результат команды неизвестен. Из него запрещено повторно выставлять заявку до reconciliation.

## 4. Переходы цикла

| Текущее состояние | Событие | Проверки | Следующее состояние | Действие |
|---|---|---|---|---|
| `DRAFT` | snapshot approved | config valid | `PUBLISHED` | сохранить snapshot |
| `PUBLISHED` | activate | mode allows | `RECONCILING` | запросить broker state |
| `RECONCILING` | match | risk pass | `BUY_ORDERS_ACTIVE` | создать недостающие BUY |
| `BUY_ORDERS_ACTIVE` | BUY fill | execution unique | `POSITION_ACCUMULATING` | рассчитать SELL |
| `POSITION_ACCUMULATING` | deeper BUY fill | execution unique | то же | заменить SELL |
| `POSITION_ACCUMULATING` | partial SELL | qty valid | `EXIT_PARTIALLY_FILLED` | уменьшить ladder inventory |
| `EXIT_PARTIALLY_FILLED` | deeper BUY fill | qty valid | `POSITION_ACCUMULATING` | пересчитать SELL по net inventory |
| любое активное | SELL fully filled | exit condition met | `CLOSING` | отменить оставшиеся BUY |
| `CLOSING` | all expected cancelled | broker match | `CLOSED` | зафиксировать результат |
| любое активное | pause | always | `PAUSED` | запретить новые команды |
| любое активное | mismatch | cannot explain | `RECONCILIATION_REQUIRED` | уведомить оператора |

## 5. Расчёт желаемого состояния

### BUY

- Для каждого незакрытого уровня не более одной активной BUY-заявки.
- Цена и количество берутся из расчёта Strategy Engine и проходят broker metadata validation.
- Все уровни имеют `TIME_IN_FORCE_DAY`.
- После завершения торгового дня отсутствующие заявки создаются только после reconciliation следующей сессии.

### SELL

- До первого BUY-исполнения SELL не создаётся.
- Активен не более один логический выход на цикл.
- Выход определяется самым глубоким уровнем, имеющим фактическое исполнение.
- Количество ограничивается свободным ladder inventory и доступной позицией брокера.
- При `sell_scope=ladder_only` базовая позиция исключается из доступного количества.
- Если контрольное `preview_sell_qty` больше разрешённого количества, заявка блокируется или уменьшается согласно принятому бизнес-решению; молчаливое превышение запрещено.

## 6. Partial fill

```text
ladder_inventory = sum(BUY executions) - sum(SELL executions)
max_sell_qty = min(ladder_inventory, broker_free_position - protected_base_qty)
```

Для количества учитывается лотность. Расчёт выполняется по исполнениям, а не по первоначальному размеру заявки.

## 7. Идемпотентность

Ключ логической команды:

```text
strategy_id / cycle_id / level_no / side / command_revision
```

Из него создаётся стабильный UUID длиной не более 36 символов для API. Перед сетевым вызовом команда сохраняется в БД со статусом `PERSISTED`.

Повторная доставка события:

- execution с известным broker execution ID → `NOOP`;
- order state с меньшей версией/временем → игнорировать;
- тот же config revision и hash → `NOOP`;
- тот же revision и другой hash → блокировать.

## 8. Reconciliation matrix

| Локально | У брокера | Решение |
|---|---|---|
| `SUBMITTING/UNKNOWN` | заявка найдена по request ID | связать и продолжить |
| `SUBMITTING/UNKNOWN` | не найдена после безопасного окна | ручная проверка или контролируемый retry тем же ключом |
| `NEW` | отсутствует | запросить state/history; не создавать замену сразу |
| отсутствует | активная заявка с известным request ID | восстановить локальную запись |
| отсутствует | неизвестная активная заявка | `RECONCILIATION_REQUIRED` |
| количество расходится | позиции/операции объясняют | применить пропущенные executions |
| количество расходится | объяснения нет | `RECONCILIATION_REQUIRED` |

## 9. Завершение цикла

Цикл закрывается только когда:

- целевой SELL исполнен согласно политике выхода;
- все оставшиеся заявки цикла отменены или достоверно отсутствуют;
- брокерская позиция согласована с protected base и net ladder inventory;
- нет команд в `UNKNOWN`;
- итоговый audit event сохранён.
