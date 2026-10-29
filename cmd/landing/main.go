// The landing server deliberately loads only public configuration.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	"videonote-captions/internal/landing"
)

func loadConfig() (landing.Config, string) {
	c := landing.Config{BaseURL: os.Getenv("BASE_URL"), BotUsername: os.Getenv("BOT_USERNAME"), FreeNotes: 3, NoteCredits: 10, Packages: [3]landing.Package{{Credits: 100, PriceRub: 99}, {Credits: 300, PriceRub: 249}, {Credits: 1000, PriceRub: 749}}}
	c.Indexable, _ = strconv.ParseBool(os.Getenv("SITE_INDEXABLE"))
	c.FreeNotes = number("FREE_VIDEO_NOTES_PER_MONTH", c.FreeNotes, 0)
	c.NoteCredits = number("VIDEO_NOTE_CREDITS", c.NoteCredits, 1)
	for i := range c.Packages {
		prefix := "CREDIT_PACKAGE_" + strconv.Itoa(i+1)
		c.Packages[i].Credits = number(prefix+"_AMOUNT", c.Packages[i].Credits, 1)
		c.Packages[i].PriceRub = number(prefix+"_PRICE", c.Packages[i].PriceRub, 1)
	}
	port := number("PORT", 8080, 1)
	if port > 65535 {
		port = 8080
	}
	return c, strconv.Itoa(port)
}
func number(key string, fallback, minimum int) int {
	n, err := strconv.Atoi(os.Getenv(key))
	if err != nil || n < minimum {
		return fallback
	}
	return n
}
func main() {
	c, port := loadConfig()
	server := &http.Server{Addr: ":" + port, Handler: landing.NewHandler(c), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("landing shutdown: %v", err)
		}
	}()
	log.Printf("landing listening on :%s", port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
