# videonote-captions

Telegram-бот: отправьте или перешлите **видеокружочек** и получите его обратно
с субтитрами. В каждый момент видно **одно слово**, в интервале его произнесения.
В паузах субтитров нет. Полная расшифровка приходит отдельным сообщением.

Проект выделен из [telegram-voice-video-bot](https://github.com/chuikoffru/telegram-voice-video-bot)
с сохранением Git-истории. Обработка голосовых, создание горизонтальных/вертикальных
роликов, аватарки и пользовательские фоны удалены.

## Запуск

Нужны Docker Compose, отдельный Telegram Bot Token и ключ OpenAI с доступом к Whisper.

```sh
cp .env.example .env
# Заполните BOT_TOKEN, OPENAI_API_KEY и POSTGRES_PASSWORD.
docker compose up -d --build
docker compose logs -f bot
```

`POSTGRES_PASSWORD` должен быть длинным случайным паролем из URL-safe символов.
База не публикует порт на хост. Данные сохраняются в томе `postgres_data`.
При недоступной базе бот завершает запуск: биллинг не отключается молча.
Схема создаётся через GORM AutoMigrate. Это новая база; перенос данных SQLite
из исходного бота не выполняется.

Для локального запуска: Go 1.25.6+, FFmpeg с libass/libx264, Noto fonts,
Postgres 16. Экспортируйте переменные окружения из `.env` и выполните `go run .`.
Сам бинарник `.env` не читает. Входящий HTTP-порт не требуется: используется
Telegram long polling. Не запускайте несколько экземпляров с одним BOT_TOKEN.

## Обработка

1. Регистрация пользователя, атомарное резервирование бесплатной обработки или кредитов.
2. Скачивание кружочка, извлечение звука FFmpeg.
3. OpenAI Whisper (`whisper-1`): текст и начало/конец каждого слова.
4. Отдельное ASS-событие для каждого слова; пересечения интервалов обрезаются.
5. FFmpeg накладывает субтитры, сохраняет звук и создаёт совместимый H.264 MP4.
6. Отправка кружочка и полной расшифровки; удаление временных файлов.

Аудио передаётся OpenAI. Модель может ошибаться в тексте и времени слов;
отсутствующие метки не заменяются искусственным равномерным распределением текста.

## Кредиты и управление

По умолчанию: 3 бесплатных кружочка за календарный месяц (Europe/Moscow),
далее 10 кредитов за кружочек. Администратор обрабатывает бесплатно.
При обычной ошибке обработки/отправки резерв возвращается. При аварийном завершении
процесса между резервированием и отправкой требуется ручная сверка операции;
распределённая очередь и автоматическое восстановление задач пока не реализованы.

- `/start`, `/help` — описание и условия.
- `/buy` — баланс и пополнение через Telegram Payments / ЮKassa.
- `/usage` — статистика (ADMIN_ID).
- `/broadcast` — рассылка (ADMIN_ID).

Пакеты по умолчанию: 100 кредитов / 99 ₽, 300 / 249 ₽, 1000 / 749 ₽.
Настройки находятся в `.env.example`. Ожидающие платежи хранятся в Postgres,
начисление выполняется транзакционно с проверкой пользователя, валюты и суммы,
повторный successful_payment не начисляет деньги дважды.

## Проверка

```sh
go test ./...
go vet ./...
go build ./...
# Отдельная пустая тестовая Postgres-база, не production:
TEST_DATABASE_URL='postgres://user:password@localhost:5432/test?sslmode=disable' go test -race ./...
```

Интеграционный тест базы проверяет повторные платежи, переживание перезапуска,
конкурентное списание и возврат лимита/кредитов. Без TEST_DATABASE_URL он пропускается.

Реальный запуск требует токенов, проверки кружочка в Telegram и тестового платежа.

## Dokploy с отдельной базой

Укажите Compose Path `./docker-compose.dokploy.yml`. В Environment задайте
`POSTGRES_URI` — внутренний URI отдельного Postgres-сервиса Dokploy, а также
`BOT_TOKEN`, `OPENAI_API_KEY` и остальные настройки из `.env.example`.
Бот подключается к внешней сети `dokploy-network`; второй Postgres не создаётся.
`POSTGRES_URI` передаётся приложению как `DATABASE_URL`.

## RU/EN website and source tracking

The Dokploy stack also runs a separate `web` service from `Dockerfile.web`.
It has no database access or bot credentials. Set public environment values:

```env
BOT_USERNAME=AudioVisualizerBot
SITE_BASE_URL=http://your-temporary-host
SITE_INDEXABLE=false
```

The temporary site stays `noindex`. For the permanent HTTPS domain set its full
URL in `SITE_BASE_URL`, then enable `SITE_INDEXABLE=true` and redeploy. Language
pages are `/ru/` and `/en/`, with localized metadata, canonical/hreflang,
`robots.txt` and `sitemap.xml`. Shared pricing variables keep site and bot in sync.
Run locally with `BOT_USERNAME=AudioVisualizerBot go run ./cmd/landing` (port 8080).
The round demo is an illustration, not a customer recording.

Bot commands: `/language ru`, `/language en`; captions retain the spoken language.
Links such as `https://t.me/AudioVisualizerBot?start=en_home_hero` record first and
latest source in Postgres with a start-event history. Payloads allow only 1–64
ASCII letters, digits, `_` and `-`; empty starts record `direct`. Language hints
`ru_` and `en_` select the interface language. `/sources` is admin-only and reports
users, activated users and paying users by first source. Existing users lacking
tracked attribution appear as `unknown`; conversions before the first recorded
start do not count toward a newly recorded source. Repeated conversions are
counted once per user. Sources are self-reported link labels, not authenticated
proof that a visitor came from a search engine. No external analytics identifier
is configured automatically.
