# Контракт Google Sheets

## 1. Назначение

Текущая вкладка `Портфель — лесенки` остаётся пользовательским представлением. Для автоматизации добавляются стабильные вкладки `BOT_CONTROL` и `BOT_CONFIG`.

Сервис не читает формулы по адресам вида `G7` и не зависит от визуальных блоков, объединённых ячеек или порядка пользовательских колонок.

## 2. Правила публикации

1. Пользователь редактирует входные параметры и preview.
2. Для новой конфигурации задаётся новый целочисленный `revision`.
3. Все строки `BOT_CONFIG` этой публикации получают одинаковую ревизию.
4. В `BOT_CONTROL` устанавливаются `published_revision=<revision>` и `approved=true`.
5. Loader читает обе вкладки повторно и принимает snapshot только при согласованной ревизии.
6. Snapshot получает hash нормализованного содержимого.
7. Повторное содержимое с тем же revision, но другим hash считается ошибкой `REVISION_MUTATED`.

## 3. `BOT_CONTROL`

Формат key/value:

| key | Тип | Пример | Назначение |
|---|---|---|---|
| `schema_version` | integer | `1` | Версия контракта |
| `published_revision` | integer | `12` | Активная публикация |
| `approved` | boolean | `TRUE` | Разрешение применить snapshot |
| `mode` | enum | `sandbox` | `disabled/dry_run/sandbox/live` |
| `pause_new_orders` | boolean | `FALSE` | Остановка новых команд |
| `account_alias` | string | `main` | Несекретный alias счёта |
| `max_order_rub` | decimal | `15000` | Максимум одной заявки |
| `max_total_reserve_rub` | decimal | `140000` | Общий резерв стратегий |
| `max_daily_turnover_rub` | decimal | `200000` | Ограничение оборота |
| `max_open_orders` | integer | `20` | Ограничение открытых заявок |
| `config_valid_until` | timestamp | `2026-09-01T00:00:00Z` | Защита от забытой конфигурации |
| `comment` | string | `MVP sandbox` | Операторское пояснение |

`account_id`, токены и credentials в таблице запрещены.

## 4. `BOT_CONFIG`

Одна строка соответствует одному уровню стратегии.

| Поле | Тип | Обязательно | Описание |
|---|---|---:|---|
| `revision` | integer | да | Ревизия публикации |
| `strategy_id` | string | да | Стабильный ID, например `sber-main` |
| `enabled` | boolean | да | Активность стратегии |
| `instrument_uid` | UUID/string | да | Идентификатор T‑Invest |
| `ticker` | string | да | Человекочитаемый ticker |
| `level_no` | integer | да | Номер 1–6 |
| `base_position_qty` | integer | да | Исходное количество бумаг |
| `base_position_avg_price` | decimal | да | Исходная средняя цена |
| `sell_scope` | enum | да | `ladder_only/entire_position` |
| `step_budget_rub` | decimal | да | Бюджет уровня |
| `entry_correction` | decimal | да | Коррекция цены входа |
| `exit_correction` | decimal | да | Коррекция цены выхода |
| `preview_buy_price` | decimal | да | Контрольный расчёт таблицы |
| `preview_buy_lots` | integer | да | Контрольный расчёт таблицы |
| `preview_sell_price` | decimal | да | Контрольный расчёт выхода |
| `preview_sell_qty` | integer | да | Контрольное количество продажи |
| `max_level_amount_rub` | decimal | да | Защитный предел уровня |
| `auto_restart` | boolean | да | В MVP всегда `FALSE` |
| `comment` | string | нет | Пояснение |

## 5. Нормализация

Перед hash и сравнением:

- строки trim;
- ticker приводится к uppercase;
- boolean допускает только `TRUE/FALSE`;
- decimal парсится независимо от локали таблицы;
- даты переводятся в RFC3339 UTC;
- строки сортируются по `strategy_id`, затем `level_no`;
- пустая строка не равна нулю;
- денежные значения переводятся в nano units без `float64`.

## 6. Валидационные инварианты

- `(revision, strategy_id, level_no)` уникален.
- В одной стратегии один `instrument_uid`.
- `level_no` начинается с 1 и не имеет пропусков.
- `preview_buy_price[n+1] < preview_buy_price[n]`.
- `step_budget_rub > 0`.
- `preview_buy_lots > 0`.
- `max_level_amount_rub >= preview_buy_price * preview_buy_lots * lot`.
- `auto_restart=false` для MVP.
- `sell_scope=entire_position` блокируется до принятия ADR.
- Нельзя публиковать `live` с просроченным `config_valid_until`.

## 7. Контрольные значения текущей таблицы

Эти данные используются только как golden fixture и должны быть повторно экспортированы перед live.

| Ticker | BUY-цены по уровням | BUY-количество по уровням |
|---|---|---|
| SBER | 275,48; 261,71; 246,55; 228,20; 203,77; 168,00 | 21; 27; 36; 47; 58; 89 |
| TRNFP | 1 028,40; 976,98; 920,41; 851,96; 760,85; 627,45 | 4; 5; 6; 8; 10; 14 |
| BTBR | 114,46; 108,74; 102,44; 94,81; 84,65; 81,00 | 34; 45; 68; 73; 94; 111 |

Минимальные golden-проверки первого уровня:

- SBER: бюджет 6 000 ₽ → цена 275,48 ₽ → 21 лот.
- TRNFP: бюджет 5 000 ₽ → цена 1 028,40 ₽ → 4 лота.
- BTBR: бюджет 4 000 ₽ → цена 114,46 ₽ → 34 лота.

## 8. Ошибки контракта

При любой ошибке новый snapshot не применяется. Активная стратегия продолжает работать по предыдущему snapshot, кроме случаев истечения `config_valid_until` или явной паузы. Ошибка публикуется в статусе и уведомлении с указанием строки и поля, но без секретов.
