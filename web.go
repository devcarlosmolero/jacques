package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"time"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var pageTemplate = template.Must(template.New("page.html").Funcs(template.FuncMap{
	"safe": func(s string) template.HTML {
		return template.HTML(s)
	},
	"date": func(t time.Time) string {
		return t.Format("Jan 2, 2006")
	},
	"datetime": func(t time.Time) string {
		return t.Format("Jan 2, 2006 · 15:04")
	},
}).ParseFS(templateFS, "templates/page.html"))

func NewWeb(store *Store) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	mux.HandleFunc("GET /t/{id}", func(w http.ResponseWriter, r *http.Request) {
		page, err := store.GetUnroll(r.PathValue("id"))
		if err != nil {
			log.Printf("loading unroll %s: %v", r.PathValue("id"), err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if page == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pageTemplate.Execute(w, page); err != nil {
			log.Printf("rendering unroll %s: %v", page.RootID, err)
		}
	})

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>jacques</title></head><body><p>jacques is a Mastodon bot. Reply <code>@jacques unroll</code> to any post in a thread.</p></body></html>`))
	})

	return mux
}
