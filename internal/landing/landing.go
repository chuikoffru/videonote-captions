// Package landing serves the public, database-free product website.
package landing

import (
	"embed"
	"encoding/xml"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

//go:embed templates/*.html assets/*
var files embed.FS
var pages = template.Must(template.ParseFS(files, "templates/*.html"))
var username = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

type Package struct{ Credits, PriceRub int }
type Config struct {
	BaseURL, BotUsername   string
	Indexable              bool
	FreeNotes, NoteCredits int
	Packages               [3]Package
}
type item struct{ Title, Text string }
type page struct {
	Config
	Lang, Other, Path, Alternate, Canonical, RU, EN, Root                                                 string
	Title, Description, Heading, Intro, Eyebrow, CTA, Disabled                                            string
	StepsTitle, DifferenceTitle, Difference, PriceTitle, PriceIntro, FAQTitle                             string
	Steps, FAQ                                                                                            []item
	Privacy, Terms, Switch, Demo, DemoText, FreeLabel, CreditLabel, PackageLabel, FooterTitle, FooterText string
	HeroLink, FooterLink, SecondaryLink                                                                   string
	Kind, LegalTitle                                                                                      string
	Legal                                                                                                 []item
}

func normalized(c Config) Config {
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(u.Host, "<>\" ") {
		c.BaseURL = ""
		c.Indexable = false
	} else {
		c.BaseURL = strings.TrimSuffix(u.String(), "/")
	}
	c.BotUsername = strings.TrimPrefix(strings.TrimSpace(c.BotUsername), "@")
	if !username.MatchString(c.BotUsername) {
		c.BotUsername = ""
	}
	if c.FreeNotes < 0 {
		c.FreeNotes = 0
	}
	if c.NoteCredits <= 0 {
		c.NoteCredits = 10
	}
	return c
}
func NewHandler(c Config) http.Handler {
	c = normalized(c)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		if !c.Indexable {
			w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", 405)
			return
		}
		path := r.URL.Path
		if path == "/ru" || path == "/en" {
			http.Redirect(w, r, path+"/", 301)
			return
		}
		switch path {
		case "/healthz":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "ok")
			return
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			if c.Indexable {
				fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: %s/sitemap.xml\n", c.BaseURL)
			} else {
				fmt.Fprint(w, "User-agent: *\nDisallow: /\n")
			}
			return
		case "/sitemap.xml":
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)
			if c.Indexable {
				for _, p := range []string{"/", "/ru/", "/en/", "/ru/privacy/", "/en/privacy/", "/ru/terms/", "/en/terms/"} {
					fmt.Fprint(w, "<url><loc>")
					_ = xml.EscapeText(w, []byte(c.BaseURL+p))
					fmt.Fprint(w, "</loc></url>")
				}
			}
			fmt.Fprint(w, "</urlset>")
			return
		}
		if path == "/assets/site.css" || path == "/assets/demo.svg" {
			data, _ := files.ReadFile(strings.TrimPrefix(path, "/"))
			if strings.HasSuffix(path, "css") {
				w.Header().Set("Content-Type", "text/css; charset=utf-8")
			} else {
				w.Header().Set("Content-Type", "image/svg+xml")
			}
			w.Header().Set("Cache-Control", "public, max-age=3600")
			w.Write(data)
			return
		}
		lang := "en"
		kind := "home"
		suffix := ""
		switch path {
		case "/":
			kind = "chooser"
		case "/ru/":
			lang = "ru"
		case "/en/":
		case "/ru/privacy/":
			lang = "ru"
			kind = "legal"
			suffix = "privacy/"
		case "/en/privacy/":
			kind = "legal"
			suffix = "privacy/"
		case "/ru/terms/":
			lang = "ru"
			kind = "legal"
			suffix = "terms/"
		case "/en/terms/":
			kind = "legal"
			suffix = "terms/"
		default:
			http.NotFound(w, r)
			return
		}
		p := localized(c, lang)
		p.Kind = kind
		p.Path = path
		p.Canonical = c.BaseURL + path
		p.Root = c.BaseURL + "/"
		p.RU = c.BaseURL + "/ru/" + suffix
		p.EN = c.BaseURL + "/en/" + suffix
		p.Alternate = "/" + p.Other + "/" + suffix
		if kind == "chooser" {
			p.Title = "Telegram video note captions · Выберите язык / Choose your language"
			p.Description = "Word-by-word captions for Telegram video notes. Субтитры на кружочки Telegram."
		}
		if kind == "legal" {
			legalCopy(&p, suffix)
			p.Title = p.LegalTitle + " · Video Note Captions"
			p.Description = p.LegalTitle + " — Video Note Captions"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.ExecuteTemplate(w, "page", p); err != nil {
			return
		}
	})
}
func localized(c Config, lang string) page {
	p := page{Config: c, Lang: lang, Other: "ru", Switch: "Русский", Privacy: "Privacy", Terms: "Terms", Demo: "Illustrative demo", DemoText: "Meet me here tomorrow", FreeLabel: "free video notes per month", CreditLabel: "credits per video note", PackageLabel: "credits", Title: "Telegram Video Note Captions — Word by Word", Description: "Add word-by-word captions to Telegram video notes. Forward a round video message to the bot and receive it with text timed to speech.", Heading: "Your video notes. Every word, seen.", Eyebrow: "WORD-BY-WORD CAPTIONS · TELEGRAM", Intro: "Forward a Telegram video note to the bot. Get your round video back with captions that appear as you speak.", CTA: "Add captions in Telegram", Disabled: "The Telegram link is temporarily unavailable. Please try again later.", StepsTitle: "From your voice to their screen.", DifferenceTitle: "Your words stay in the picture.", Difference: "A transcript lives below a message. Captions live inside it. Your video note comes back with words timed to the original speech, plus a separate transcript.", PriceTitle: "A small note. A clear price.", PriceIntro: "Try the monthly free allowance, then use credits. Prices are in Russian rubles (RUB); payment availability depends on your country and payment provider. The bot shows the final invoice before payment.", FAQTitle: "A few things to know.", FooterTitle: "Let your next note be read, too.", FooterText: "Your voice sets the pace. The words follow."}
	p.Steps = []item{{"Send your video note", "Record or forward a round video message in the Telegram bot."}, {"Let the words follow", "The bot recognises the audio and times each word to your speech."}, {"Share the result", "Receive the captioned video note and a separate transcript in Telegram."}}
	p.FAQ = []item{{"What can I send?", "Telegram video notes: the round video messages. Regular video files and voice messages are not supported by this workflow."}, {"Does it translate my speech?", "No. Captions follow the original spoken language. The language of the website and bot interface does not translate your recording."}, {"Can recognition make mistakes?", "Yes. Background noise, names and unclear speech can affect recognition. Check the result before sharing it."}, {"Where is my audio processed?", "The bot downloads the video note and sends extracted audio to OpenAI for speech recognition. Temporary local media are removed after the processing attempt. Telegram and the recognition provider have their own data policies."}, {"How do free notes and credits work?", "The monthly free allowance is used first. After that, each processed video note uses the number of credits shown above. Check your current allowance and balance in the bot."}}
	if lang == "ru" {
		p.Other = "en"
		p.Switch = "English"
		p.Privacy = "Приватность"
		p.Terms = "Условия"
		p.Demo = "Иллюстрация результата"
		p.DemoText = "Давай увидимся здесь завтра"
		p.FreeLabel = russianNotes(c.FreeNotes) + " бесплатно в месяц"
		p.CreditLabel = "кредитов за кружочек"
		p.PackageLabel = "кредитов"
		p.Title = "Субтитры на кружочки Telegram — по одному слову"
		p.Description = "Добавьте субтитры к видеокружочку прямо в Telegram. Одно слово появляется в момент речи. Перешлите кружочек боту и получите готовое видео."
		p.Heading = "Субтитры на кружочки Telegram — по одному слову"
		p.Eyebrow = "ВАШ ГОЛОС. ТЕПЕРЬ И В ТЕКСТЕ."
		p.Intro = "Перешлите видеокружочек боту. Получите его обратно с текстом, который появляется в момент речи."
		p.CTA = "Добавить субтитры в Telegram"
		p.Disabled = "Ссылка на бота временно недоступна. Попробуйте зайти позже."
		p.StepsTitle = "Сказать. Отправить. Быть понятым."
		p.DifferenceTitle = "Слова остаются в кадре."
		p.Difference = "Расшифровка — это текст под сообщением. Субтитры — текст внутри него. Вы получите кружочек со словами в ритме вашей речи и отдельную расшифровку."
		p.PriceTitle = "Маленький кружочек. Понятная цена."
		p.PriceIntro = "Начните с бесплатного месячного лимита. Дальше — кредиты без подписки. Цены в рублях; окончательная сумма будет в счёте в Telegram до оплаты."
		p.FAQTitle = "Осталось несколько вопросов?"
		p.FooterTitle = "Пусть следующий кружочек увидят и прочитают."
		p.FooterText = "Вы задаёте ритм. Слова следуют за вами."
		p.Steps = []item{{"Отправьте кружочек", "Запишите или перешлите круглое видеосообщение в чат с ботом."}, {"Дайте словам появиться", "Бот распознает аудио и синхронизирует слова с вашей речью."}, {"Поделитесь результатом", "Получите готовый кружочек с субтитрами и отдельную расшифровку в Telegram."}}
		p.FAQ = []item{{"Что можно отправить?", "Видеокружочки Telegram — круглые видеосообщения. Обычные видеофайлы и голосовые сообщения в этом сценарии не поддерживаются."}, {"Это перевод на другой язык?", "Нет. Субтитры остаются на языке исходной речи. Язык сайта и интерфейса бота не меняет язык записи."}, {"Может ли бот ошибиться?", "Да. Шум, имена и нечёткая речь влияют на распознавание. Проверьте текст перед публикацией."}, {"Куда передаётся аудио?", "Бот скачивает кружочек и отправляет извлечённое аудио в OpenAI для распознавания речи. Временные локальные медиа удаляются после попытки обработки. У Telegram и провайдера распознавания свои правила обработки данных."}, {"Как работают бесплатный лимит и кредиты?", "Сначала расходуется бесплатный месячный лимит. После него каждый обработанный кружочек стоит указанное выше число кредитов. Текущий остаток и баланс можно посмотреть в боте."}}
	}
	if c.BotUsername != "" {
		p.HeroLink = "https://t.me/" + c.BotUsername + "?start=" + lang + "_home_hero"
		p.FooterLink = "https://t.me/" + c.BotUsername + "?start=" + lang + "_home_footer"
		p.SecondaryLink = "https://t.me/" + c.BotUsername + "?start=" + lang + "_home_pricing"
	}
	return p
}
func legalCopy(p *page, kind string) {
	if kind == "privacy/" {
		p.LegalTitle = "Privacy and data processing"
		p.Legal = []item{{"What the bot processes", "The bot receives your Telegram user ID, language preference, video note and associated message information. It stores account balances, usage, payments and start-source information to operate the service."}, {"Speech recognition", "Audio extracted from your video note is sent to OpenAI for transcription. This processing is not entirely on your device. Telegram and OpenAI apply their own privacy and retention policies."}, {"Temporary files and records", "Local input video, extracted audio and rendered output are scheduled for deletion after the processing attempt. This does not delete copies in Telegram or at external providers. Service records such as balance, usage and payment history are stored in the service database. No universal retention period is promised."}, {"Your choices and contact", "Only send recordings you are entitled to process. Do not send sensitive or confidential recordings unless you accept the external processing described here. Review this information before sending a recording."}}
	} else {
		p.LegalTitle = "Terms of use"
		p.Legal = []item{{"The service", "The bot adds word-by-word captions to Telegram video notes and returns a transcript. Recognition can be inaccurate; review the result before publishing. Translation and regular video file support are not included."}, {"Credits and payment", "The free monthly allowance, processing cost and credit packages are shown on the homepage and in the bot. Check the final invoice before paying. Payments are handled through Telegram and the payment provider; international availability depends on the provider and country. No subscription or guaranteed processing time is offered."}, {"Your recordings", "You are responsible for having permission to send and process a recording and to share the resulting media. Do not use the service to infringe others’ rights."}, {"Service availability and questions", "Processing depends on Telegram, the speech provider and service availability. No uninterrupted availability or perfect recognition is guaranteed. Keep the payment confirmation and transaction identifier in case a payment needs to be checked."}}
	}
	if p.Lang != "ru" {
		return
	}
	if kind == "privacy/" {
		p.LegalTitle = "Приватность и обработка данных"
		p.Legal = []item{{"Какие данные обрабатывает бот", "Бот получает ваш Telegram ID, язык, видеокружочек и связанные сведения о сообщении. Для работы сервиса сохраняются баланс, использование, платежи и источник запуска бота."}, {"Распознавание речи", "Извлечённое аудио передаётся в OpenAI для распознавания. Обработка происходит не только на вашем устройстве. У Telegram и OpenAI действуют собственные правила приватности и хранения данных."}, {"Временные файлы и записи", "Локальные исходное видео, аудио и готовое видео удаляются после попытки обработки. Это не удаляет копии у Telegram или внешних провайдеров. Баланс, история использования и платежей хранятся в базе сервиса. Единый срок хранения всех данных не заявляется."}, {"Ваш выбор и связь", "Отправляйте записи, на обработку которых у вас есть право. Учитывайте передачу аудио внешнему провайдеру перед отправкой конфиденциальных записей. Учитывайте эти условия перед отправкой записи."}}
	} else {
		p.LegalTitle = "Условия использования"
		p.Legal = []item{{"Что делает сервис", "Бот добавляет субтитры по одному слову на видеокружочки Telegram и возвращает расшифровку. Распознавание может ошибаться: проверяйте результат перед публикацией. Перевод и обработка обычных видеофайлов не входят в этот сценарий."}, {"Кредиты и оплата", "Бесплатный месячный лимит, стоимость обработки и пакеты кредитов указаны на главной странице и в боте. Проверьте итоговый счёт до оплаты. Платежи проходят через Telegram и платёжного провайдера. Доступность зависит от страны и провайдера. Подписка и гарантированный срок обработки не предлагаются."}, {"Ваши записи", "Вы отвечаете за наличие разрешения на отправку, обработку и публикацию записей. Не используйте сервис для нарушения прав других людей."}, {"Доступность и вопросы", "Обработка зависит от Telegram, провайдера распознавания и доступности сервиса. Безошибочное распознавание и непрерывная работа не гарантируются. Сохраняйте подтверждение и идентификатор платежа на случай, если потребуется его проверка."}}
	}
}

func russianNotes(n int) string {
	if n%100 >= 11 && n%100 <= 14 {
		return "кружочков"
	}
	switch n % 10 {
	case 1:
		return "кружочек"
	case 2, 3, 4:
		return "кружочка"
	default:
		return "кружочков"
	}
}
