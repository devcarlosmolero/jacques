package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Fatalf("%s must be an integer, got %q", key, v)
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

	auto := AutoConfig{
		Enabled:   envOr("JACQUES_AUTO_UNROLL", "on") != "off",
		MinPosts:  envInt("JACQUES_AUTO_UNROLL_MIN_POSTS", 5),
		Quiet:     time.Duration(envInt("JACQUES_AUTO_UNROLL_QUIET_MINUTES", 15)) * time.Minute,
		HourlyCap: envInt("JACQUES_AUTO_UNROLL_HOURLY_CAP", 4),
		Retention: time.Duration(envInt("JACQUES_AUTO_UNROLL_RETENTION_DAYS", 7)) * 24 * time.Hour,
	}

	log.Printf("jacques v%s", botVersion())
	bot := NewBot(NewClient(server, token), store, strings.TrimRight(baseURL, "/"), auto)
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
