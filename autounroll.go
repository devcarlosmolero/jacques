package main

import (
	"context"
	"fmt"
	"html"
	"log"
	"strings"
	"time"
)

// AutoConfig controls jacques' proactive thread unrolling: he watches the
// federated timeline for self-reply chains and, once a thread is long enough
// and has gone quiet, replies to it with a link to the unrolled page.
type AutoConfig struct {
	Enabled   bool
	MinPosts  int           // threads shorter than this are never announced
	Quiet     time.Duration // wait this long after the last post before announcing
	HourlyCap int           // never announce more than this many threads per hour
	Retention time.Duration // observation data older than this is pruned
}

// pollTimeline reads new public timeline statuses and records self-reply
// chains as auto-unroll candidates. On the very first run it only sets the
// cursor so jacques doesn't replay old history after a fresh deploy.
func (b *Bot) pollTimeline(ctx context.Context) {
	cursor, err := b.store.GetMeta("last_timeline_id")
	if err != nil {
		log.Printf("reading timeline cursor: %v", err)
		return
	}
	statuses, err := b.client.TimelinePublic(ctx, cursor)
	if err != nil {
		log.Printf("fetching timeline: %v", err)
		return
	}
	if len(statuses) == 0 {
		return
	}
	if cursor == "" {
		if err := b.store.SetMeta("last_timeline_id", statuses[0].ID); err != nil {
			log.Printf("saving timeline cursor: %v", err)
		}
		return
	}
	for i := len(statuses) - 1; i >= 0; i-- {
		b.observe(ctx, statuses[i])
		if err := b.store.SetMeta("last_timeline_id", statuses[i].ID); err != nil {
			log.Printf("saving timeline cursor: %v", err)
		}
	}
}

// observe checks whether a timeline status extends a self-reply chain and, if
// so, tracks the thread. Authors who are bots or carry #nobot in their bio
// are left alone.
func (b *Bot) observe(ctx context.Context, s Status) {
	if s.Account.ID == b.me.ID || s.Account.Bot {
		return
	}
	if s.InReplyToID == nil || s.InReplyToAccountID == nil || *s.InReplyToAccountID != s.Account.ID {
		return
	}
	if hasNoBot(s.Account.Note) {
		return
	}
	if optedOut, err := b.store.IsOptedOut(s.Account.ID); err != nil {
		log.Printf("checking opt-out: %v", err)
		return
	} else if optedOut {
		return
	}

	rootID, depth, ok, err := b.store.GetThreadPost(*s.InReplyToID)
	if err != nil {
		log.Printf("looking up thread post: %v", err)
		return
	}
	if ok {
		depth++
	} else {
		sc, err := b.client.Context(ctx, s.ID)
		if err != nil {
			log.Printf("fetching context for %s: %v", s.ID, err)
			return
		}
		if len(sc.Ancestors) == 0 {
			return
		}
		// Only pure self-threads qualify: an author replying to themselves
		// inside someone else's conversation is not a thread to announce.
		for _, a := range sc.Ancestors {
			if a.Account.ID != s.Account.ID {
				return
			}
		}
		rootID = sc.Ancestors[0].ID
		depth = len(sc.Ancestors)
		for i, a := range sc.Ancestors {
			if err := b.store.SaveThreadPost(a.ID, rootID, i); err != nil {
				log.Printf("saving thread post: %v", err)
			}
		}
	}
	if err := b.store.SaveThreadPost(s.ID, rootID, depth); err != nil {
		log.Printf("saving thread post: %v", err)
		return
	}
	if err := b.store.TrackAutoThread(rootID, s.Account.Acct, depth+1, s.ID, s.CreatedAt); err != nil {
		log.Printf("tracking thread %s: %v", rootID, err)
	}
}

// announceDue unrolls and replies to every tracked thread that has reached
// the minimum length and gone quiet, within the hourly cap.
func (b *Bot) announceDue(ctx context.Context) {
	due, err := b.store.DueAutoThreads(b.auto.MinPosts, time.Now().Add(-b.auto.Quiet))
	if err != nil {
		log.Printf("listing due threads: %v", err)
		return
	}
	for _, t := range due {
		n, err := b.store.AnnouncedCountSince(time.Now().Add(-time.Hour))
		if err != nil {
			log.Printf("counting announcements: %v", err)
			return
		}
		if n >= b.auto.HourlyCap {
			return
		}
		if err := b.announce(ctx, t); err != nil {
			log.Printf("announcing thread %s: %v", t.RootID, err)
		}
		// Mark even on failure: a thread that can't be announced once is
		// retried forever otherwise, and a stale announcement is worse
		// than none.
		if err := b.store.MarkAnnounced(t.RootID, time.Now()); err != nil {
			log.Printf("marking thread %s announced: %v", t.RootID, err)
		}
	}
}

func (b *Bot) announce(ctx context.Context, t AutoThread) error {
	if existing, err := b.store.GetUnroll(t.RootID); err != nil {
		return err
	} else if existing != nil {
		return nil // someone already asked for this one; its link is in the thread
	}

	root, err := b.client.GetStatus(ctx, t.RootID)
	if err != nil {
		return err
	}
	// Re-check opt-outs with fresh data: the bio may have changed or a
	// "forget me" may have arrived since the thread was first seen.
	if root.Account.Bot || hasNoBot(root.Account.Note) {
		return nil
	}
	if optedOut, err := b.store.IsOptedOut(root.Account.ID); err != nil {
		return err
	} else if optedOut {
		return nil
	}
	if root.Visibility != "public" && root.Visibility != "unlisted" {
		return nil
	}

	rootCtx, err := b.client.Context(ctx, root.ID)
	if err != nil {
		return err
	}
	chain := buildChain(root, rootCtx.Descendants)
	if len(chain) < b.auto.MinPosts {
		return nil // posts were deleted, or the count was off
	}

	page := b.buildPage(root, chain)
	if err := b.store.SaveUnroll(page, "jacques"); err != nil {
		return err
	}
	last := chain[len(chain)-1]
	log.Printf("auto-unrolled thread %s by @%s (%d posts)", root.ID, root.Account.Acct, len(chain))
	// Announcements are the one thing jacques says in public: they only
	// work as propagation if passers-by can see them.
	return b.client.Reply(ctx, &last, "public", fmt.Sprintf(
		"@%s I unrolled this thread (%d posts) into a single page for easier reading: %s\n\nI'm a bot. Reply \"unroll\" to any thread and I'll do the same; add #nobot to your bio or send me a private mention saying \"forget me\" and I'll leave you alone.",
		root.Account.Acct, len(chain), b.pageURL(root.ID)))
}

// hasNoBot reports whether an account bio opts out via the #nobot convention.
// Mastodon serves the note as HTML where hashtags render as
// <a ...>#<span>nobot</span></a>, so tags are stripped before matching.
func hasNoBot(note string) bool {
	text := strings.ToLower(html.UnescapeString(tagRe.ReplaceAllString(note, "")))
	return strings.Contains(text, "#nobot")
}
