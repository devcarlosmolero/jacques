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
CREATE TABLE IF NOT EXISTS thread_posts (
	status_id TEXT PRIMARY KEY,
	root_id   TEXT NOT NULL,
	depth     INTEGER NOT NULL,
	seen_at   TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS auto_threads (
	root_id        TEXT PRIMARY KEY,
	author_acct    TEXT NOT NULL,
	post_count     INTEGER NOT NULL,
	last_status_id TEXT NOT NULL,
	last_post_at   TEXT NOT NULL,
	announced_at   TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS optouts (
	account_id TEXT PRIMARY KEY,
	acct       TEXT NOT NULL,
	created_at TEXT NOT NULL
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

type AutoThread struct {
	RootID     string
	AuthorAcct string
	PostCount  int
}

func (s *Store) GetThreadPost(statusID string) (rootID string, depth int, ok bool, err error) {
	err = s.db.QueryRow(`SELECT root_id, depth FROM thread_posts WHERE status_id = ?`, statusID).Scan(&rootID, &depth)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	return rootID, depth, err == nil, err
}

func (s *Store) SaveThreadPost(statusID, rootID string, depth int) error {
	_, err := s.db.Exec(`INSERT INTO thread_posts (status_id, root_id, depth, seen_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(status_id) DO NOTHING`, statusID, rootID, depth, time.Now().UTC().Format(time.RFC3339))
	return err
}

// Prune drops observation data older than the cutoff so the tables don't
// grow without bound. Announced threads are safe to drop too: the unroll
// page saved in the unrolls table is what prevents a second announcement.
// If a pruned thread comes back to life, observe rebuilds its state from a
// single context fetch.
func (s *Store) Prune(cutoff time.Time) error {
	c := cutoff.UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`DELETE FROM thread_posts WHERE seen_at < ?`, c); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM auto_threads WHERE last_post_at < ?`, c)
	return err
}

func (s *Store) TrackAutoThread(rootID, authorAcct string, postCount int, lastStatusID string, lastPostAt time.Time) error {
	_, err := s.db.Exec(`INSERT INTO auto_threads (root_id, author_acct, post_count, last_status_id, last_post_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(root_id) DO UPDATE SET
			post_count     = MAX(post_count, excluded.post_count),
			last_status_id = excluded.last_status_id,
			last_post_at   = excluded.last_post_at`,
		rootID, authorAcct, postCount, lastStatusID, lastPostAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) DueAutoThreads(minPosts int, quietSince time.Time) ([]AutoThread, error) {
	rows, err := s.db.Query(`SELECT root_id, author_acct, post_count FROM auto_threads
		WHERE announced_at = '' AND post_count >= ? AND last_post_at <= ?
		ORDER BY last_post_at`,
		minPosts, quietSince.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var due []AutoThread
	for rows.Next() {
		var t AutoThread
		if err := rows.Scan(&t.RootID, &t.AuthorAcct, &t.PostCount); err != nil {
			return nil, err
		}
		due = append(due, t)
	}
	return due, rows.Err()
}

func (s *Store) MarkAnnounced(rootID string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE auto_threads SET announced_at = ? WHERE root_id = ?`,
		at.UTC().Format(time.RFC3339), rootID)
	return err
}

func (s *Store) AnnouncedCountSince(t time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM auto_threads WHERE announced_at > ?`,
		t.UTC().Format(time.RFC3339)).Scan(&n)
	return n, err
}

func (s *Store) OptOut(accountID, acct string) error {
	_, err := s.db.Exec(`INSERT INTO optouts (account_id, acct, created_at) VALUES (?, ?, ?)
		ON CONFLICT(account_id) DO NOTHING`,
		accountID, acct, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) OptIn(accountID string) error {
	_, err := s.db.Exec(`DELETE FROM optouts WHERE account_id = ?`, accountID)
	return err
}

func (s *Store) IsOptedOut(accountID string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM optouts WHERE account_id = ?`, accountID).Scan(&n)
	return n > 0, err
}

// ForgetPendingThreads drops every tracked-but-unannounced thread by an
// author, so opting out also erases what jacques had gathered about them.
func (s *Store) ForgetPendingThreads(authorAcct string) error {
	_, err := s.db.Exec(`DELETE FROM auto_threads WHERE author_acct = ? AND announced_at = ''`, authorAcct)
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
