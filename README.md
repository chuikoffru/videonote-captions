# videonote-captions

Telegram-бот: отправьте или перешлите **видеокружочек** и получите его обратно
с субтитрами. В каждый момент видно **одно слово**, в интервале его произнесения.
В паузах субтитров нет. Полная расшифровка приходит отдельным сообщением.

При включённой функции можно прислать также **обычное видео** из галереи или
видеодокумент и выбрать «Сделать кружочек» либо «Кружочек с субтитрами».
Простая конвертация бесплатна: до **5 успешных результатов в день** на пользователя,
обновление в полночь Europe/Moscow. Это отдельный лимит, не расходующий субтитры
или кредиты. Вариант с субтитрами использует действующий месячный лимит и цену;
отдельного списания за преобразование формы нет.

## Видео в кружочек

Включение после приёмки: `ENABLE_VIDEO_TO_CIRCLE=true` для bot и web.
По умолчанию функция выключена. `PLAIN_CIRCLES_PER_DAY=5` задаёт дневной лимит.
В Dokploy web получает те же публичные значения из Compose; bot читает `.env`.
При отключённом флаге новый сценарий и его реклама скрыты; прежние кружочки
по-прежнему сразу получают субтитры.

- Личные чаты; видео MP4/MOV/WebM и другие поддержанные контейнеры FFmpeg,
  в том числе видеодокументы. Содержимое проверяется ffprobe.
- Вход до 20 MiB и 600 секунд; выход до 60 секунд, 384×384, H.264/AAC.
- Кроп по центру с учётом поворота. Для длинного видео требуется отдельное
  подтверждение использования первых 60 секунд. Произвольный таймкод и ручной
  выбор кадра пока отсутствуют. HDR требует предварительного SDR-экспорта.
- Видео без звука работает в простом режиме. Если речь для субтитров не найдена,
  резерв возвращается и предлагается отдельный бесплатный вариант без текста.
- Меню привязано к пользователю, чату и исходнику, живёт 30 минут; после
  перезапуска нужно прислать файл заново. Повторный клик не запускает дубль.
- Максимум две параллельные обработки, таймаут 180 секунд. Каталог задания
  удаляется при завершении. Простая конвертация не передаёт аудио в OpenAI.

Настройки: `MAX_INPUT_VIDEO_BYTES=20971520` (не выше лимита облачного Bot API),
`MAX_INPUT_VIDEO_SECONDS=600`, `VIDEO_PROCESS_TIMEOUT_SECONDS=180`.
Обычная ошибка возвращает резерв; при аварии процесса незавершённые дневные
резервы требуют сверки, как и существующие резервы субтитров. Автоматической
повторной отправки при неоднозначном ответе Telegram нет.

`/usage` разделяет обычные кружочки и субтитры; `/sources` показывает общую
активацию и активацию субтитров отдельно. Факт покупки определяется платежами,
а не использованием бесплатной конвертации.

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
