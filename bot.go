package main

import (
	"context"
	"fmt"
	"html"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
)

type Bot struct {
	client    *Client
	store     *Store
	baseURL   string
	policy    *bluemonday.Policy
	auto      AutoConfig
	me        Account
	lastPrune time.Time
}

func NewBot(client *Client, store *Store, baseURL string, auto AutoConfig) *Bot {
	return &Bot{
		client:  client,
		store:   store,
		baseURL: baseURL,
		policy:  newPolicy(),
		auto:    auto,
	}
}

func newPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class").
		Matching(regexp.MustCompile(`^(?:(?:mention|hashtag|ellipsis|invisible|u-url|h-card)\s*)+$`)).
		OnElements("a", "span")
	p.RequireNoFollowOnLinks(true)
	return p
}

func (b *Bot) Run(ctx context.Context) error {
	me, err := b.client.VerifyCredentials(ctx)
	if err != nil {
		return fmt.Errorf("verifying credentials: %w", err)
	}
	b.me = me
	log.Printf("logged in as @%s", me.Acct)

	since, err := b.store.GetMeta("last_notification_id")
	if err != nil {
		return err
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		notifications, err := b.client.Mentions(ctx, since)
		if err != nil {
			log.Printf("fetching notifications: %v", err)
			continue
		}
		for i := len(notifications) - 1; i >= 0; i-- {
			n := notifications[i]
			b.handle(ctx, n)
			since = n.ID
			if err := b.store.SetMeta("last_notification_id", since); err != nil {
				log.Printf("saving notification cursor: %v", err)
			}
		}

		if b.auto.Enabled {
			b.pollTimeline(ctx)
			b.announceDue(ctx)
			if time.Since(b.lastPrune) > time.Hour {
				if err := b.store.Prune(time.Now().Add(-b.auto.Retention)); err != nil {
					log.Printf("pruning observation data: %v", err)
				}
				b.lastPrune = time.Now()
			}
		}
	}
}

func (b *Bot) handle(ctx context.Context, n Notification) {
	if n.Type != "mention" || n.Status == nil {
		return
	}
	if n.Status.Account.ID == b.me.ID {
		return
	}
	if n.Status.Visibility == "direct" {
		from := n.Status.Account
		if hasPhrase(n.Status.Content, "forget", "me") {
			log.Printf("opt-out requested by @%s", from.Acct)
			if err := b.store.OptOut(from.ID, from.Acct); err != nil {
				log.Printf("saving opt-out: %v", err)
				return
			}
			if err := b.store.ForgetPendingThreads(from.Acct); err != nil {
				log.Printf("forgetting pending threads: %v", err)
			}
			b.replyf(ctx, n.Status, "@%s done. I've dropped what I'd gathered about your threads and won't unroll you on my own again. Message me \"remember me\" if you change your mind.", from.Acct)
			return
		}
		if hasPhrase(n.Status.Content, "remember", "me") {
			log.Printf("opt-in requested by @%s", from.Acct)
			if err := b.store.OptIn(from.ID); err != nil {
				log.Printf("removing opt-out: %v", err)
				return
			}
			b.replyf(ctx, n.Status, "@%s welcome back! I'll happily unroll your threads again.", from.Acct)
			return
		}
	}
	if !hasUnrollCommand(n.Status.Content) {
		if n.Status.Visibility == "public" || n.Status.Visibility == "unlisted" {
			log.Printf("boosting mention by @%s on status %s", n.Status.Account.Acct, n.Status.ID)
			if err := b.client.Reblog(ctx, n.Status.ID); err != nil {
				log.Printf("reblog failed: %v", err)
			}
		}
		return
	}
	log.Printf("unroll requested by @%s on status %s", n.Status.Account.Acct, n.Status.ID)
	if err := b.unroll(ctx, n.Status); err != nil {
		log.Printf("unroll failed: %v", err)
		b.replyf(ctx, n.Status, "@%s sorry, I couldn't unroll that thread.", n.Status.Account.Acct)
	}
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

func hasUnrollCommand(content string) bool {
	text := html.UnescapeString(tagRe.ReplaceAllString(content, " "))
	for _, field := range strings.Fields(text) {
		if strings.EqualFold(strings.Trim(field, ".,!?:;"), "unroll") {
			return true
		}
	}
	return false
}

// hasPhrase reports whether the given words appear consecutively in the
// status content, ignoring case, punctuation and HTML markup.
func hasPhrase(content string, words ...string) bool {
	text := html.UnescapeString(tagRe.ReplaceAllString(content, " "))
	var fields []string
	for _, f := range strings.Fields(text) {
		fields = append(fields, strings.ToLower(strings.Trim(f, `.,!?:;"'`)))
	}
	for i := 0; i+len(words) <= len(fields); i++ {
		match := true
		for j, w := range words {
			if fields[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func (b *Bot) unroll(ctx context.Context, mention *Status) error {
	if mention.InReplyToID == nil {
		return b.replyf(ctx, mention, "@%s reply to a post inside a thread and I'll unroll it for you.", mention.Account.Acct)
	}

	mentionCtx, err := b.client.Context(ctx, mention.ID)
	if err != nil {
		return err
	}
	if len(mentionCtx.Ancestors) == 0 {
		return b.replyf(ctx, mention, "@%s I couldn't find the thread this belongs to.", mention.Account.Acct)
	}
	root := mentionCtx.Ancestors[0]

	if existing, err := b.store.GetUnroll(root.ID); err != nil {
		return err
	} else if existing != nil {
		return b.replyf(ctx, mention, "@%s this thread is already unrolled (%d posts): %s",
			mention.Account.Acct, existing.PostCount, b.pageURL(root.ID))
	}

	rootCtx, err := b.client.Context(ctx, root.ID)
	if err != nil {
		return err
	}
	chain := buildChain(root, rootCtx.Descendants)

	page := b.buildPage(root, chain)
	if err := b.store.SaveUnroll(page, mention.Account.Acct); err != nil {
		return err
	}
	return b.replyf(ctx, mention, "@%s here you go, %d posts by @%s unrolled: %s",
		mention.Account.Acct, len(chain), root.Account.Acct, b.pageURL(root.ID))
}

func buildChain(root Status, descendants []Status) []Status {
	byParent := make(map[string][]Status)
	for _, d := range descendants {
		if d.InReplyToID != nil {
			byParent[*d.InReplyToID] = append(byParent[*d.InReplyToID], d)
		}
	}
	chain := []Status{root}
	current := root.ID
	for {
		var next *Status
		for i, candidate := range byParent[current] {
			if candidate.Account.ID != root.Account.ID {
				continue
			}
			if next == nil || candidate.CreatedAt.Before(next.CreatedAt) {
				next = &byParent[current][i]
			}
		}
		if next == nil {
			return chain
		}
		chain = append(chain, *next)
		current = next.ID
	}
}

func (b *Bot) buildPage(root Status, chain []Status) *PageData {
	page := &PageData{
		RootID:    root.ID,
		RootURL:   root.URL,
		Author:    root.Account,
		PostCount: len(chain),
		StartedAt: root.CreatedAt,
	}
	for _, status := range chain {
		post := PagePost{
			HTML:      b.policy.Sanitize(status.Content),
			URL:       status.URL,
			CreatedAt: status.CreatedAt,
		}
		for _, m := range status.MediaAttachments {
			if m.Type == "image" {
				post.Media = append(post.Media, m)
			}
		}
		page.Posts = append(page.Posts, post)
	}
	return page
}

func (b *Bot) pageURL(rootID string) string {
	return b.baseURL + "/t/" + rootID
}

// replyf answers with a private mention: whatever jacques has to say to a
// requester stays between the two of them. Only automatic announcements
// (see announce) are public.
func (b *Bot) replyf(ctx context.Context, to *Status, format string, args ...any) error {
	return b.client.Reply(ctx, to, "direct", fmt.Sprintf(format, args...))
}
