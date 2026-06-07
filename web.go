package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
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
	"inc": func(i int) int {
		return i + 1
	},
}).ParseFS(templateFS, "templates/page.html"))

type pageView struct {
	*PageData
	BaseURL string
}

func (v pageView) PageURL() string {
	return v.BaseURL + "/t/" + v.RootID
}

func NewWeb(store *Store, baseURL string) http.Handler {
	baseURL = strings.TrimRight(baseURL, "/")
	mux := http.NewServeMux()

	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	loadUnroll := func(w http.ResponseWriter, r *http.Request, id string) *PageData {
		page, err := store.GetUnroll(id)
		if err != nil {
			log.Printf("loading unroll %s: %v", id, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return nil
		}
		if page == nil {
			http.NotFound(w, r)
			return nil
		}
		return page
	}

	mux.HandleFunc("GET /t/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if md, ok := strings.CutSuffix(id, ".md"); ok {
			page := loadUnroll(w, r, md)
			if page == nil {
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprint(w, renderMarkdown(page))
			return
		}
		page := loadUnroll(w, r, id)
		if page == nil {
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pageTemplate.Execute(w, pageView{PageData: page, BaseURL: baseURL}); err != nil {
			log.Printf("rendering unroll %s: %v", page.RootID, err)
		}
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		fmt.Fprintf(w, `{"status":"ok","version":%q}`, botVersion())
	})

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>jacques</title></head><body><p>jacques is a Mastodon bot. Reply <code>@jacques unroll</code> to any post in a thread.</p></body></html>`))
	})

	return mux
}
