package handlers

var english = map[string]string{
	"👋 Отправь кружочек — я добавлю субтитры: одно слово в момент его произнесения. /help — условия, /buy — баланс и кредиты.":                                                                                                                                                          "👋 Send a video note and I will add captions, one word as it is spoken. /help — details, /buy — balance and credits. /language ru|en — language.",
	"Отправь или перешли видеокружочек. Я верну его с субтитрами — по одному слову, синхронно с речью — и полным текстом под ним.\n\nБесплатно: %d кружочка в месяц. Далее: %d кредитов за кружочек. /buy — баланс и пополнение.\n\nЗвук отправляется в OpenAI для распознавания речи.": "Send or forward a video note. I will return it with captions, one word at a time, synchronized with speech, and the full transcript below. Captions stay in the original speech language.\n\nFree: %d video notes per month. After that: %d credits per video note. /buy — balance and top up. /language ru|en — language.\n\nAudio is sent to OpenAI for speech recognition.",
	"ℹ️ Используйте команду так:\n`/broadcast Текст сообщения`\nили ответьте на сообщение командой `/broadcast`":                                                                                                                                                                        "ℹ️ Use:\n`/broadcast Message text`\nor reply to a message with `/broadcast`",
	"❌ Ошибка получения списка пользователей.":         "❌ Could not load the user list.",
	"🚀 Начинаю рассылку для %d пользователей...":       "🚀 Sending to %d users...",
	"✅ Рассылка завершена!\n\nУспешно: %d\nОшибок: %d": "✅ Broadcast complete!\n\nSent: %d\nFailed: %d",
	"❌ База данных не инициализирована.":               "❌ Database is unavailable.",
	"❌ Ошибка получения статистики.":                   "❌ Could not load statistics.",
	"📊 Статистика\nПользователей: %d\nБез видео: %d\nОдно видео: %d\nБольше одного: %d\nКружочков: %d\nБесплатных: %d\nПлатных: %d\nСумма балансов: %d кредитов\nПотрачено: %d кредитов": "📊 Statistics\nUsers: %d\nNo videos: %d\nOne video: %d\nMore than one: %d\nVideo notes: %d\nFree: %d\nPaid: %d\nTotal balances: %d credits\nSpent: %d credits",
	"❌ <b>Недостаточно кредитов</b>\n\n":                         "❌ <b>Not enough credits</b>\n\n",
	"💰 Нужно: %d %s\n":                                           "💰 Required: %d %s\n",
	"💳 У вас: %d %s\n\n":                                         "💳 You have: %d %s\n\n",
	"🛒 <b>Выберите пакет для пополнения:</b>":                    "🛒 <b>Choose a top-up package:</b>",
	"🪙 %d кредитов — %d ₽":                                       "🪙 %d credits — %d RUB",
	"🪙 %d кредитов — %d ₽ (выгода!)":                             "🪙 %d credits — %d RUB (better value!)",
	"🪙 %d кредитов — %d ₽ (лучшая цена!)":                        "🪙 %d credits — %d RUB (best price!)",
	"🎬 Бесплатные кружочки в этом месяце закончились!\n\n":       "🎬 Your free video notes for this month have been used!\n\n",
	"🔄 Лимит сбросится: %s\n":                                    "🔄 Allowance resets: %s\n",
	"💰 Стоимость 1 кружочка: %d %s\n":                            "💰 Price per video note: %d %s\n",
	"💳 Ваш баланс: %d %s\n\n":                                    "💳 Your balance: %d %s\n\n",
	"🛒 Выберите пакет для пополнения:":                           "🛒 Choose a top-up package:",
	"💰 Ваш текущий баланс: %d %s\n\n":                            "💰 Your current balance: %d %s\n\n",
	"🎬 Использовано бесплатных кружочков: %d из %d\n":            "🎬 Free video notes used: %d of %d\n",
	"🔄 Обновление лимитов: %s\n\n":                               "🔄 Allowance resets: %s\n\n",
	"🛒 Выберите пакет кредитов:":                                 "🛒 Choose a credit package:",
	"%d кредитов":                                                "%d credits",
	"%d кредитов (выгода!)":                                      "%d credits (better value!)",
	"%d кредитов (лучшая цена!)":                                 "%d credits (best price!)",
	"❌ Ошибка при создании платежа. Попробуйте позже.":           "❌ Could not create payment. Please try again later.",
	"Пополнение баланса":                                         "Credit top-up",
	"Заказ недействителен. Используйте /buy":                     "Invalid order. Please use /buy",
	"Не удалось зачислить платёж. Обратитесь в поддержку с ID: ": "Could not credit your payment. Contact support with ID: ",
	"✅ Платёж учтён. Баланс: %d кредитов":                        "✅ Payment recorded. Balance: %d credits",
	"Бот занят обработкой. Попробуйте через минуту.":             "The bot is busy. Please try again in a minute.",
	"⏳ Обрабатываю видео-сообщение...":                           "⏳ Processing your video note...",
	"⬇️ Скачиваю видео...":                                       "⬇️ Downloading video...",
	"❌ Ошибка скачивания видео.":                                 "❌ Could not download video.",
	"🔊 Извлекаю звук...":                                         "🔊 Extracting audio...",
	"❌ Ошибка обработки аудио.":                                  "❌ Could not process audio.",
	"📝 Распознаю речь...":                                        "📝 Recognizing speech...",
	"❌ Ошибка распознавания речи.":                               "❌ Could not recognize speech.",
	"⚠️ Речь не распознана или видео без слов.":                  "⚠️ No speech was recognized in this video.",
	"🎬 Накладываю субтитры...":                                   "🎬 Adding captions...",
	"❌ Ошибка создания видео с субтитрами.":                      "❌ Could not create the captioned video.",
	"📤 Отправляю результат...":                                   "📤 Sending your video...",
	"❌ Ошибка отправки результата.":                              "❌ Could not send the result.",
	"📝 Транскрипт:\n\n":                                          "📝 Transcript:\n\n",
}

func tr(lang, text string) string {
	if lang == "en" {
		if translated, ok := english[text]; ok {
			return translated
		}
	}
	return text
}
