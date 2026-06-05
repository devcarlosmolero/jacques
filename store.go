package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type PageData struct {
	RootID    string
	RootURL   string
	Author    Account
	PostCount int
	StartedAt time.Time
	Posts     []PagePost
}

type PagePost struct {
	HTML      string
	URL       string
	CreatedAt time.Time
	Media     []MediaAttachment
}

const schema = `
CREATE TABLE IF NOT EXISTS unrolls (
	root_id      TEXT PRIMARY KEY,
	author_acct  TEXT NOT NULL,
	requested_by TEXT NOT NULL DEFAULT '',
	post_count   INTEGER NOT NULL,
	data         TEXT NOT NULL,
	created_at   TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) GetMeta(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) GetUnroll(rootID string) (*PageData, error) {
	var raw string
	err := s.db.QueryRow(`SELECT data FROM unrolls WHERE root_id = ?`, rootID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var page PageData
	if err := json.Unmarshal([]byte(raw), &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (s *Store) SaveUnroll(page *PageData, requestedBy string) error {
	raw, err := json.Marshal(page)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO unrolls (root_id, author_acct, requested_by, post_count, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(root_id) DO NOTHING`,
		page.RootID, page.Author.Acct, requestedBy, page.PostCount, string(raw), time.Now().UTC().Format(time.RFC3339))
	return err
}
