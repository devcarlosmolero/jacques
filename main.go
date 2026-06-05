package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	preview := flag.Bool("preview", false, "serve a sample thread at /t/preview without connecting to Mastodon")
	flag.Parse()

	listen := envOr("JACQUES_LISTEN", ":8080")

	if *preview {
		store, err := NewStore(":memory:")
		if err != nil {
			log.Fatalf("opening database: %v", err)
		}
		if err := store.SaveUnroll(samplePage(), "preview"); err != nil {
			log.Fatalf("seeding preview: %v", err)
		}
		log.Printf("preview thread at http://localhost%s/t/preview", listen)
		log.Fatal(http.ListenAndServe(listen, NewWeb(store)))
	}

	server := os.Getenv("JACQUES_SERVER")
	token := os.Getenv("JACQUES_TOKEN")
	baseURL := os.Getenv("JACQUES_BASE_URL")
	dbPath := envOr("JACQUES_DB", "jacques.db")

	if server == "" || token == "" || baseURL == "" {
		log.Fatal("JACQUES_SERVER, JACQUES_TOKEN and JACQUES_BASE_URL must be set")
	}

	store, err := NewStore(dbPath)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer store.Close()

	log.Printf("jacques v%s", botVersion())
	bot := NewBot(NewClient(server, token), store, strings.TrimRight(baseURL, "/"))
	go func() {
		if err := bot.Run(context.Background()); err != nil {
			log.Fatalf("bot stopped: %v", err)
		}
	}()

	log.Printf("jacques serving pages on %s", listen)
	if err := http.ListenAndServe(listen, NewWeb(store)); err != nil {
		log.Fatal(err)
	}
}
