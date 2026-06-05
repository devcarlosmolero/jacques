package main

import (
	"testing"
	"time"
)

func status(id, author string, replyTo string, minute int) Status {
	s := Status{
		ID:        id,
		Account:   Account{ID: author},
		CreatedAt: time.Date(2026, 6, 6, 10, minute, 0, 0, time.UTC),
	}
	if replyTo != "" {
		s.InReplyToID = &replyTo
	}
	return s
}

func TestBuildChainFollowsAuthorSelfReplies(t *testing.T) {
	root := status("1", "alice", "", 0)
	descendants := []Status{
		status("2", "alice", "1", 1),
		status("3", "bob", "1", 2),
		status("4", "alice", "2", 3),
		status("5", "alice", "3", 4),
		status("6", "carol", "4", 5),
	}
	chain := buildChain(root, descendants)
	want := []string{"1", "2", "4"}
	if len(chain) != len(want) {
		t.Fatalf("got %d posts, want %d", len(chain), len(want))
	}
	for i, id := range want {
		if chain[i].ID != id {
			t.Errorf("chain[%d] = %s, want %s", i, chain[i].ID, id)
		}
	}
}

func TestBuildChainPicksEarliestBranch(t *testing.T) {
	root := status("1", "alice", "", 0)
	descendants := []Status{
		status("late", "alice", "1", 30),
		status("early", "alice", "1", 5),
	}
	chain := buildChain(root, descendants)
	if len(chain) != 2 || chain[1].ID != "early" {
		t.Fatalf("expected earliest self-reply, got %+v", chain)
	}
}

func TestHasNoBot(t *testing.T) {
	cases := map[string]bool{
		`<p>I post about ferns. #nobot</p>`:                                                   true,
		`<p>opt me out: <a href="https://example.com/tags/nobot">#<span>nobot</span></a></p>`: true,
		`<p>I post about ferns. #NoBot</p>`:                                                   true,
		`<p>I love robots and the nobot hashtag discourse</p>`:                                false,
		`<p>just a regular bio with a <a href="#">link</a></p>`:                               false,
		``: false,
	}
	for note, want := range cases {
		if got := hasNoBot(note); got != want {
			t.Errorf("hasNoBot(%q) = %v, want %v", note, got, want)
		}
	}
}

func TestAutoThreadLifecycle(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	if err := store.TrackAutoThread("root", "alice", 3, "s3", now); err != nil {
		t.Fatal(err)
	}
	if err := store.TrackAutoThread("root", "alice", 6, "s6", now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}

	due, err := store.DueAutoThreads(5, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("expected no due threads, got %+v", due)
	}

	due, err = store.DueAutoThreads(5, now.Add(30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].RootID != "root" || due[0].PostCount != 6 {
		t.Fatalf("expected root due with 6 posts, got %+v", due)
	}

	if err := store.MarkAnnounced("root", now.Add(31*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.TrackAutoThread("root", "alice", 8, "s8", now.Add(40*time.Minute)); err != nil {
		t.Fatal(err)
	}
	due, err = store.DueAutoThreads(5, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("expected announced thread to stay announced, got %+v", due)
	}

	n, err := store.AnnouncedCountSince(now)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("AnnouncedCountSince = %d, want 1", n)
	}
}

func TestHasPhrase(t *testing.T) {
	cases := map[string]bool{
		`<p><span class="h-card"><a href="https://example.com/@jacques">@jacques</a></span> forget me</p>`: true,
		`<p>@jacques Forget me!</p>`:        true,
		`<p>@jacques please forget me.</p>`: true,
		`<p>@jacques forget about me</p>`:   false,
		`<p>@jacques me forget</p>`:         false,
		`<p>@jacques forget</p>`:            false,
	}
	for content, want := range cases {
		if got := hasPhrase(content, "forget", "me"); got != want {
			t.Errorf("hasPhrase(%q, forget me) = %v, want %v", content, got, want)
		}
	}
}

func TestOptOutLifecycle(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	if err := store.TrackAutoThread("root", "alice@remote.tld", 6, "s6", now); err != nil {
		t.Fatal(err)
	}

	if err := store.OptOut("acc1", "alice@remote.tld"); err != nil {
		t.Fatal(err)
	}
	if err := store.ForgetPendingThreads("alice@remote.tld"); err != nil {
		t.Fatal(err)
	}

	out, err := store.IsOptedOut("acc1")
	if err != nil {
		t.Fatal(err)
	}
	if !out {
		t.Fatal("expected acc1 to be opted out")
	}
	due, err := store.DueAutoThreads(5, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("expected pending threads to be forgotten, got %+v", due)
	}

	if err := store.OptIn("acc1"); err != nil {
		t.Fatal(err)
	}
	out, err = store.IsOptedOut("acc1")
	if err != nil {
		t.Fatal(err)
	}
	if out {
		t.Fatal("expected acc1 to be opted back in")
	}
}

func TestPrune(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	old := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	fresh := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	if err := store.TrackAutoThread("stale", "alice", 2, "s2", old); err != nil {
		t.Fatal(err)
	}
	if err := store.TrackAutoThread("alive", "bob", 6, "s6", fresh); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveThreadPost("s6", "alive", 5); err != nil {
		t.Fatal(err)
	}

	if err := store.Prune(fresh.Add(-24 * time.Hour)); err != nil {
		t.Fatal(err)
	}

	due, err := store.DueAutoThreads(1, fresh.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].RootID != "alive" {
		t.Fatalf("expected only the fresh thread to survive, got %+v", due)
	}
	if _, _, ok, err := store.GetThreadPost("s6"); err != nil || !ok {
		t.Fatalf("expected fresh thread post to survive, ok=%v err=%v", ok, err)
	}
}

func TestHasUnrollCommand(t *testing.T) {
	cases := map[string]bool{
		`<p><span class="h-card"><a href="https://example.com/@jacques">@jacques</a></span> unroll</p>`: true,
		`<p><a href="https://example.com/@jacques">@jacques</a> UNROLL!</p>`:                            true,
		`<p>@jacques please unroll this</p>`:                                                            true,
		`<p>@jacques what do you think?</p>`:                                                            false,
		`<p>talking about unrolling things</p>`:                                                         false,
	}
	for content, want := range cases {
		if got := hasUnrollCommand(content); got != want {
			t.Errorf("hasUnrollCommand(%q) = %v, want %v", content, got, want)
		}
	}
}
