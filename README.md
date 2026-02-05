# karting-telegram-bot

Telegram-бот для картингового чемпионата: регистрация пилотов/команд, запись на этапы с лимитами, оплата, календарь, результаты, фото, админ-рассылки и выгрузки — **всё хранится в Google Sheets**.

## Стек
- Go 1.20+
- Telegram Bot API (long polling)
- Google Sheets API (service account)
- Встроенный HTTP-сервер для платежных вебхуков и выдачи CSV выгрузок

> Платежи сделаны через абстракцию `PaymentProvider`. В проекте есть **готовый Stub-провайдер** (для теста), и место для подключения реального провайдера (ЮKassa/CloudPayments/Tinkoff/etc).

---

## 1) Быстрый старт

### 1.1 Создай Google Sheet
Создай таблицу с вкладками (точно такие имена):
- `Participants`
- `Teams`
- `Stages`
- `Stage_Registrations`
- `Results`
- `Photos`

Заполни заголовки колонок (первую строку) как в разделе **Схема таблиц** ниже.

### 1.2 Service Account для Google Sheets
1) В Google Cloud Console включи **Google Sheets API**.
2) Создай **Service Account**, скачай JSON ключ.
3) Положи файл сюда: `./secrets/service-account.json`
4) Открой твой Google Sheet и **расшарь** его на email сервис-аккаунта (роль Editor).

### 1.3 Настрой переменные окружения
Скопируй `.env.example` в `.env`, заполни:
- `TELEGRAM_BOT_TOKEN`
- `GOOGLE_SHEETS_SPREADSHEET_ID`
- `ADMIN_TG_IDS`
- `BASE_PUBLIC_URL` (можно оставить пустым для локального теста, но ссылки на оплату будут локальные)

### 1.4 Запуск
```bash
go mod tidy
go run ./cmd/bot
```

---

## 2) Команды и сценарии

### Для участника
- `/start` — регистрация или показ профиля
- кнопки: Записаться на этап, Сменить команду, Календарь, Результаты, Фото

### Для админа
- `/admin` — панель
- Создать этап / Редактировать этап
- Открыть/Закрыть регистрацию
- Рассылка (всем / по этапу / не оплатившим / резервам)
- Выгрузить CSV списка этапа

---

## 3) Схема Google Sheets

### Participants
| tg_id | first_name | last_name | nick | team_name | created_at |

### Teams
| team_id | team_name | created_at |

### Stages
| stage_id | title | date | time | place | address | reg_open | price |

### Stage_Registrations
| stage_id | tg_id | team_name | role | pay_status | created_at |
`role`: `main` или `reserve`  
`pay_status`: `unpaid` / `paid` / `cancelled`

### Results
| stage_id | tg_id | best_time | position | points |

### Photos
| stage_id | url |

---

## 4) Лимиты команды (важно)
- На один этап: максимум **3** участника в статусе `main` на одну команду
- Все последующие автоматически становятся `reserve`

---

## 5) Платежи (как подключить реального провайдера)
Реальные платежи зависят от выбранного заказчиком провайдера.
Сейчас в проекте:
- `internal/payments/stub` — генерирует ссылку оплаты и принимает вебхук “paid”.

Чтобы подключить реального провайдера:
1) Реализуй интерфейс `PaymentProvider` (см. `internal/payments/provider.go`)
2) В `PAYMENT_PROVIDER` укажи имя провайдера
3) В `internal/payments/factory.go` добавь создание нужного клиента

---

## 6) Деплой
- Можно деплоить на VPS (systemd) или Docker.
- Для `BASE_PUBLIC_URL` нужен домен/https, если провайдер платежей требует вебхуки извне.

---

## 7) Структура проекта
- `cmd/bot` — точка входа
- `internal/bot` — диалоги и меню
- `internal/sheets` — работа с Google Sheets
- `internal/payments` — платежи (stub + интерфейс)
- `internal/admin` — админ-операции (рассылки/выгрузки/этапы)

---

## 8) Примечания
- Проект сделан так, чтобы его можно было расширять без переписывания логики.
- Все ключевые “правила” (лимиты, статусы, фильтры) вынесены в отдельные функции.
