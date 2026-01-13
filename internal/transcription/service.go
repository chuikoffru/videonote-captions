package transcription

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"

	openai "github.com/sashabaranov/go-openai"
)

// Word представляет слово с временными метками
type Word struct {
	Word  string
	Start float64
	End   float64
}

// Result представляет результат транскрипции
type Result struct {
	Text  string
	Words []Word
}

// Service предоставляет функции транскрипции
type Service struct {
	client *openai.Client
}

// New создаёт новый сервис транскрипции
func New(apiKey, proxyURL string) *Service {
	config := openai.DefaultConfig(apiKey)

	if proxyURL != "" {
		proxyParsed, err := url.Parse(proxyURL)
		if err == nil {
			config.HTTPClient = &http.Client{
				Transport: &http.Transport{
					Proxy: http.ProxyURL(proxyParsed),
				},
			}
		}
	}

	return &Service{
		client: openai.NewClientWithConfig(config),
	}
}

// TranscribeAudio транскрибирует аудиофайл с временными метками слов
func (s *Service) TranscribeAudio(ctx context.Context, audioPath string) (*Result, error) {
	file, err := os.Open(audioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	resp, err := s.client.CreateTranscription(ctx, openai.AudioRequest{
		Model:                  openai.Whisper1,
		FilePath:               audioPath,
		Format:                 openai.AudioResponseFormatVerboseJSON,
		TimestampGranularities: []openai.TranscriptionTimestampGranularity{openai.TranscriptionTimestampGranularityWord},
	})
	if err != nil {
		return nil, fmt.Errorf("transcription failed: %w", err)
	}

	result := &Result{
		Text:  resp.Text,
		Words: make([]Word, 0, len(resp.Words)),
	}

	for _, w := range resp.Words {
		result.Words = append(result.Words, Word{
			Word:  w.Word,
			Start: float64(w.Start),
			End:   float64(w.End),
		})
	}

	// Если нет слов, создаём один сегмент с полным текстом
	if len(result.Words) == 0 && result.Text != "" {
		result.Words = []Word{{
			Word:  result.Text,
			Start: 0,
			End:   5,
		}}
	}

	return result, nil
}
