package landing

import "fmt"

func conversionCopy(p *page) {
	mb := (p.MaxInputVideoBytes + 1048575) / 1048576
	p.ConversionTitle = "Turn a regular video into a Telegram circle"
	p.ConversionIntro = fmt.Sprintf("Send a video from your gallery or as a file. Choose a circle without captions, free up to %d times a day, or add word-by-word captions on the current terms.", p.PlainCirclesPerDay)
	p.ConversionTerms = fmt.Sprintf("Up to %d MB and %d seconds per upload. The circle is up to 60 seconds long. We crop the centre of the frame and ask before using the first minute of a longer video. HDR video needs to be exported in SDR. The daily allowance resets at midnight Moscow time and is separate from captions.", mb, p.MaxInputVideoSeconds)
	p.ConversionCTA = "Make a circle in Telegram"
	p.Intro += " You can also send a regular video and choose a circle with or without captions."
	p.FreeLabel = "free captioned video notes per month"
	p.CreditLabel = "credits per captioned video note"
	p.PriceIntro = "These prices apply to captions. " + p.PriceIntro
	p.FAQ[0].Text = fmt.Sprintf("Round Telegram video notes and regular videos, including video files up to %d MB and %d seconds. Ordinary videos have two choices: a free circle or a circle with captions. Voice messages are not supported.", mb, p.MaxInputVideoSeconds)
	p.FAQ[3].Text = "Media are processed on our server and temporary local files are deleted after the attempt. Audio is sent to OpenAI only when you choose captions. A circle without captions does not use speech recognition. Telegram and OpenAI have their own data policies."
	p.FAQ[4].Text = fmt.Sprintf("Circles without captions are free up to %d times per day, resetting at midnight Moscow time. Captions use a separate monthly free allowance, then the credits shown above. Errors do not consume the allowance or credits.", p.PlainCirclesPerDay)
	if p.Lang == "ru" {
		p.ConversionTitle = "Кружочек из обычного видео"
		p.ConversionIntro = fmt.Sprintf("Отправьте видео из галереи или файлом. Выберите кружочек без субтитров — бесплатно, до %d раз в день — или добавьте пословные субтитры по текущим условиям.", p.PlainCirclesPerDay)
		p.ConversionTerms = fmt.Sprintf("Исходник — до %d MB и %d секунд. Готовый кружочек — до 60 секунд. Обрезаем центр кадра; перед использованием первой минуты длинного видео спрашиваем подтверждение. HDR-видео нужно экспортировать в SDR. Дневной лимит обновляется в полночь по Москве и не расходует бесплатные субтитры.", mb, p.MaxInputVideoSeconds)
		p.ConversionCTA = "Сделать кружочек в Telegram"
		p.Intro += " Можно прислать и обычное видео — сделаем кружочек с субтитрами или без."
		p.FreeLabel = russianNotes(p.FreeNotes) + " с субтитрами бесплатно в месяц"
		p.CreditLabel = "кредитов за субтитры"
		p.PriceIntro = "Эти условия относятся к субтитрам. " + p.PriceIntro
		p.FAQ[0].Text = fmt.Sprintf("Кружочки Telegram и обычные видео, в том числе отправленные файлом: до %d MB и %d секунд. Для обычного видео можно выбрать бесплатный кружочек или кружочек с субтитрами. Голосовые сообщения не поддерживаются.", mb, p.MaxInputVideoSeconds)
		p.FAQ[3].Text = "Медиа обрабатываются на нашем сервере, временные локальные файлы удаляются после попытки обработки. Аудио отправляется в OpenAI только при выборе субтитров. Простой кружочек не использует распознавание речи. У Telegram и OpenAI действуют собственные правила обработки данных."
		p.FAQ[4].Text = fmt.Sprintf("Кружочки без субтитров бесплатны: до %d в день, обновление в полночь по Москве. Для субтитров действует отдельный месячный бесплатный лимит, затем кредиты по указанным выше условиям. Ошибки не расходуют лимит и кредиты.", p.PlainCirclesPerDay)
	}
	if p.BotUsername != "" {
		p.ConversionLink = "https://t.me/" + p.BotUsername + "?start=" + p.Lang + "_video_to_circle"
	}
}

func conversionLegal(p *page, kind string) {
	if kind == "privacy/" {
		p.Legal[0].Text = "The bot receives your Telegram user ID, language preference, uploaded video or video note and associated message information. It stores balances, daily conversion reservations, monthly caption usage, payments and start-source information to operate the service."
		p.Legal[1].Text = "Audio is sent to OpenAI only when captions are selected. A circle without captions is converted on our server without speech recognition. Telegram and OpenAI apply their own privacy and retention policies."
		if p.Lang == "ru" {
			p.Legal[0].Text = "Бот получает ваш Telegram ID, язык, загруженное видео или кружочек и связанные сведения о сообщении. Для работы сервиса сохраняются баланс, дневные резервы конвертации, месячное использование субтитров, платежи и источник запуска."
			p.Legal[1].Text = "Аудио передаётся в OpenAI только при выборе субтитров. Кружочек без субтитров конвертируется на нашем сервере без распознавания речи. У Telegram и OpenAI действуют собственные правила приватности и хранения данных."
		}
	} else {
		p.Legal[0].Text = "The bot converts ordinary videos into Telegram video notes, with optional word-by-word captions and a separate transcript. Video notes can also receive captions. Recognition may be inaccurate; review results before publishing. Translation is not included. The bot shows input limits and asks before trimming longer videos."
		p.Legal[1].Text = fmt.Sprintf("Circles without captions are free up to %d per day, resetting at midnight Moscow time. Captions use the existing monthly allowance and credits shown in the bot. Errors return the reservation. Payments and credit packages follow the terms shown in the final Telegram invoice.", p.PlainCirclesPerDay)
		if p.Lang == "ru" {
			p.Legal[0].Text = "Бот превращает обычное видео в кружочек Telegram, по выбору добавляет пословные субтитры и отдельную расшифровку. На готовые кружочки тоже можно добавить субтитры. Распознавание может ошибаться: проверяйте результат перед публикацией. Перевод не входит в услугу. Бот показывает ограничения исходника и спрашивает подтверждение перед обрезкой длинного видео."
			p.Legal[1].Text = fmt.Sprintf("Кружочки без субтитров бесплатны: до %d в день, обновление в полночь по Москве. Субтитры используют существующий месячный лимит и кредиты по условиям в боте. При ошибках резерв возвращается. Пакеты кредитов и оплата — по условиям итогового счёта в Telegram.", p.PlainCirclesPerDay)
		}
	}
}
