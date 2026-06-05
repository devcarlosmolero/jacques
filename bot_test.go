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
