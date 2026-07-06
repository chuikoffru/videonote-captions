package httpclient

import (
	"net/http"
	"net/url"
	"time"
)

// shared — общий HTTP-клиент для скачивания файлов из Telegram.
// По умолчанию работает напрямую; при вызове Init с PROXY — через прокси.
var shared = &http.Client{Timeout: 60 * time.Second}

// Init настраивает общий клиент. Если proxyURL не пустой, все запросы
// (в т.ч. скачивание файлов с api.telegram.org) идут через прокси —
// это нужно там, где Telegram недоступен напрямую.
func Init(proxyURL string) error {
	if proxyURL == "" {
		return nil
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return err
	}
	shared.Transport = &http.Transport{
		Proxy: http.ProxyURL(parsed),
	}
	return nil
}

// Get выполняет GET-запрос через общий клиент.
func Get(rawURL string) (*http.Response, error) {
	return shared.Get(rawURL)
}
