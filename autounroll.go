package main

import (
	"context"
	"html"
	"log"
	"strings"
	"time"
)

type AutoConfig struct {
	Enabled   bool
	MinPosts  int
	Quiet     time.Duration
	HourlyCap int
	Retention time.Duration
}

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
		if err := b.store.MarkAnnounced(t.RootID, time.Now()); err != nil {
			log.Printf("marking thread %s announced: %v", t.RootID, err)
		}
	}
}

func (b *Bot) announce(ctx context.Context, t AutoThread) error {
	if existing, err := b.store.GetUnroll(t.RootID); err != nil {
		return err
	} else if existing != nil {
		return nil
	}

	root, err := b.client.GetStatus(ctx, t.RootID)
	if err != nil {
		return err
	}
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
		return nil
	}

	page := b.buildPage(root, chain)
	if err := b.store.SaveUnroll(page, "jacques"); err != nil {
		return err
	}
	last := chain[len(chain)-1]
	log.Printf("auto-unrolled thread %s by @%s (%d posts)", root.ID, root.Account.Acct, len(chain))
	return b.replyPublicf(ctx, &last,
		"@%s I unrolled this thread (%d posts) into a single page for easier reading: %s\n\nI'm a bot. Reply \"unroll\" to any thread and I'll do the same; add #nobot to your bio or mention me saying \"forget me\" and I'll leave you alone.",
		root.Account.Acct, len(chain), b.pageURL(root.ID))
}

func hasNoBot(note string) bool {
	text := strings.ToLower(html.UnescapeString(tagRe.ReplaceAllString(note, "")))
	return strings.Contains(text, "#nobot")
}
