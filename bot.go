package main

import (
	"context"
	"fmt"
	"html"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
)

type Bot struct {
	client       *Client
	store        *Store
	baseURL      string
	policy       *bluemonday.Policy
	auto         AutoConfig
	monthlyStats bool
	me           Account
	lastPrune    time.Time
}

func NewBot(client *Client, store *Store, baseURL string, auto AutoConfig, monthlyStats bool) *Bot {
	return &Bot{
		client:       client,
		store:        store,
		baseURL:      baseURL,
		policy:       newPolicy(),
		auto:         auto,
		monthlyStats: monthlyStats,
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

		b.deliverReminders(ctx)
		b.celebrateBirthdays(ctx)

		if b.monthlyStats {
			b.maybePostMonthlyStats(ctx)
		}
	}
}

func (b *Bot) birthday(ctx context.Context, mention *Status) error {
	from := mention.Account
	if hasWord(mention.Content, "forget") || hasWord(mention.Content, "remove") {
		if err := b.store.RemoveBirthday(from.ID); err != nil {
			return err
		}
		log.Printf("birthday removed by @%s", from.Acct)
		return b.replyf(ctx, mention, "@%s done, I've forgotten your birthday.", from.Acct)
	}
	month, day, ok := parseBirthday(mention.Content)
	if !ok {
		return b.replyf(ctx, mention, "@%s tell me the date, like \"birthday June 12\" or \"birthday 12 June\". Say \"birthday forget\" and I'll drop it.", from.Acct)
	}
	if err := b.store.SetBirthday(from.ID, from.Acct, month, day); err != nil {
		return err
	}
	log.Printf("birthday set by @%s: %s %d", from.Acct, month, day)
	return b.replyf(ctx, mention, "@%s noted! I'll wish you a happy birthday every %s %d. Say \"birthday forget\" if you change your mind.", from.Acct, month, day)
}

func (b *Bot) celebrateBirthdays(ctx context.Context) {
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	last, err := b.store.GetMeta("birthday_date")
	if err != nil {
		log.Printf("reading birthday cursor: %v", err)
		return
	}
	if last == today {
		return
	}
	accts, err := b.store.BirthdaysOn(now.Month(), now.Day())
	if err != nil {
		log.Printf("listing birthdays: %v", err)
		return
	}
	if now.Month() == time.February && now.Day() == 28 && !isLeapYear(now.Year()) {
		leapAccts, err := b.store.BirthdaysOn(time.February, 29)
		if err != nil {
			log.Printf("listing leap birthdays: %v", err)
			return
		}
		accts = append(accts, leapAccts...)
	}
	for _, acct := range accts {
		text := fmt.Sprintf("a little raccoon told me it's @%s's birthday today. Happy birthday!", acct)
		if err := b.client.Post(ctx, "public", text); err != nil {
			log.Printf("posting birthday wish for @%s: %v", acct, err)
			return
		}
		log.Printf("wished @%s a happy birthday", acct)
	}
	if err := b.store.SetMeta("birthday_date", today); err != nil {
		log.Printf("saving birthday cursor: %v", err)
	}
}

func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

func parseBirthday(content string) (time.Month, int, bool) {
	fields := cleanFields(content)
	idx := -1
	for i, f := range fields {
		if f == "birthday" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, 0, false
	}
	var month time.Month
	day := 0
	for _, f := range fields[idx+1:] {
		if m, ok := monthByName(f); ok && month == 0 {
			month = m
			continue
		}
		if n, err := strconv.Atoi(trimOrdinal(f)); err == nil && day == 0 {
			day = n
		}
	}
	if month == 0 || day < 1 || day > 31 {
		return 0, 0, false
	}
	if time.Date(2024, month, day, 0, 0, 0, 0, time.UTC).Day() != day {
		return 0, 0, false
	}
	return month, day, true
}

func trimOrdinal(word string) string {
	for _, suffix := range []string{"st", "nd", "rd", "th"} {
		if trimmed, ok := strings.CutSuffix(word, suffix); ok {
			return trimmed
		}
	}
	return word
}

func monthByName(word string) (time.Month, bool) {
	if len(word) < 3 {
		return 0, false
	}
	for m := time.January; m <= time.December; m++ {
		name := strings.ToLower(m.String())
		if word == name || word == name[:3] {
			return m, true
		}
	}
	return 0, false
}

func (b *Bot) remind(ctx context.Context, mention *Status) error {
	d, ok := parseRemindDuration(mention.Content)
	if !ok {
		return b.replyf(ctx, mention, "@%s tell me when, like \"remind me in 3 days\", \"remind me in 2 hours\" or \"remind me tomorrow\".", mention.Account.Acct)
	}
	at := time.Now().UTC().Add(d)
	if err := b.store.AddReminder(mention.ID, mention.Account.Acct, mention.Visibility, at); err != nil {
		return err
	}
	log.Printf("reminder set by @%s for %s", mention.Account.Acct, at.Format(time.RFC3339))
	return b.replyf(ctx, mention, "@%s got it, I'll nudge you here around %s UTC.", mention.Account.Acct, at.Format("Jan 2, 2006 15:04"))
}

func (b *Bot) deliverReminders(ctx context.Context) {
	due, err := b.store.DueReminders(time.Now())
	if err != nil {
		log.Printf("listing due reminders: %v", err)
		return
	}
	for _, r := range due {
		if err := b.replyf(ctx, &Status{ID: r.StatusID, Visibility: r.Visibility}, "@%s you asked me to remind you about this. Here it is!", r.Acct); err != nil {
			log.Printf("delivering reminder %d to @%s: %v", r.ID, r.Acct, err)
		}
		if err := b.store.DeleteReminder(r.ID); err != nil {
			log.Printf("deleting reminder %d: %v", r.ID, err)
		}
	}
}

func parseRemindDuration(content string) (time.Duration, bool) {
	fields := cleanFields(content)
	for i, f := range fields {
		if f == "remind" {
			return parseWhen(fields[i+1:])
		}
	}
	return 0, false
}

func parseWhen(fields []string) (time.Duration, bool) {
	for i, f := range fields {
		if f == "tomorrow" {
			return 24 * time.Hour, true
		}
		n := 0
		switch f {
		case "a", "an", "next":
			n = 1
		default:
			v, err := strconv.Atoi(f)
			if err != nil || v <= 0 {
				continue
			}
			n = v
		}
		if i+1 >= len(fields) {
			continue
		}
		unit, ok := unitDuration(fields[i+1])
		if !ok {
			continue
		}
		d := time.Duration(n) * unit
		if d < time.Minute || d > 366*24*time.Hour {
			return 0, false
		}
		return d, true
	}
	return 0, false
}

func unitDuration(word string) (time.Duration, bool) {
	switch strings.TrimSuffix(word, "s") {
	case "minute", "min":
		return time.Minute, true
	case "hour", "hr":
		return time.Hour, true
	case "day":
		return 24 * time.Hour, true
	case "week":
		return 7 * 24 * time.Hour, true
	case "month":
		return 30 * 24 * time.Hour, true
	}
	return 0, false
}

func (b *Bot) maybePostMonthlyStats(ctx context.Context) {
	now := time.Now().UTC()
	month := now.Format("2006-01")
	last, err := b.store.GetMeta("stats_month")
	if err != nil {
		log.Printf("reading stats cursor: %v", err)
		return
	}
	if last == month {
		return
	}
	if last == "" {
		if err := b.store.SetMeta("stats_month", month); err != nil {
			log.Printf("saving stats cursor: %v", err)
		}
		return
	}

	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	prevStart := monthStart.AddDate(0, -1, 0)
	threads, posts, err := b.store.UnrollStats(prevStart, monthStart)
	if err != nil {
		log.Printf("computing monthly stats: %v", err)
		return
	}
	total, err := b.store.TotalUnrolls()
	if err != nil {
		log.Printf("counting unrolls: %v", err)
		return
	}

	var text string
	if threads == 0 {
		text = fmt.Sprintf("monthly rummage report: a quiet %s, no new threads unrolled. %d unrolled all time.\n\nReply \"unroll\" to any post in a thread and I'll lay the whole thing out on a single page.",
			prevStart.Format("January"), total)
	} else {
		text = fmt.Sprintf("monthly rummage report: in %s I unrolled %d threads (%d posts dragged into the open). %d unrolled all time.\n\nReply \"unroll\" to any post in a thread and I'll lay the whole thing out on a single page.",
			prevStart.Format("January"), threads, posts, total)
	}
	if err := b.client.Post(ctx, "public", text); err != nil {
		log.Printf("posting monthly stats: %v", err)
		return
	}
	log.Printf("posted monthly stats for %s", prevStart.Format("2006-01"))
	if err := b.store.SetMeta("stats_month", month); err != nil {
		log.Printf("saving stats cursor: %v", err)
	}
}

func (b *Bot) handle(ctx context.Context, n Notification) {
	if n.Type != "mention" || n.Status == nil {
		return
	}
	if n.Status.Account.ID == b.me.ID {
		return
	}
	if hasWord(n.Status.Content, "birthday") {
		if err := b.birthday(ctx, n.Status); err != nil {
			log.Printf("birthday failed: %v", err)
		}
		return
	}
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
	if n.Status.InReplyToID != nil {
		isPrompt, err := b.store.IsHelpPrompt(*n.Status.InReplyToID)
		if err != nil {
			log.Printf("checking help prompt: %v", err)
		} else if isPrompt {
			if err := b.store.DeleteHelpPrompt(*n.Status.InReplyToID); err != nil {
				log.Printf("clearing help prompt: %v", err)
			}
			b.helpSelection(ctx, n.Status)
			return
		}
	}
	cmd := parseCommand(n.Status.Content)
	if cmd == "" {
		if n.Status.Visibility == "public" || n.Status.Visibility == "unlisted" {
			log.Printf("boosting mention by @%s on status %s", n.Status.Account.Acct, n.Status.ID)
			if err := b.client.Reblog(ctx, n.Status.ID); err != nil {
				log.Printf("reblog failed: %v", err)
			}
		}
		return
	}
	log.Printf("%s requested by @%s on status %s", cmd, n.Status.Account.Acct, n.Status.ID)
	var err error
	switch cmd {
	case "unroll":
		if err = b.unroll(ctx, n.Status); err != nil {
			b.replyf(ctx, n.Status, "@%s sorry, I couldn't unroll that thread.", n.Status.Account.Acct)
		}
	case "refresh":
		if err = b.refresh(ctx, n.Status); err != nil {
			b.replyf(ctx, n.Status, "@%s sorry, I couldn't refresh that thread.", n.Status.Account.Acct)
		}
	case "remind":
		if err = b.remind(ctx, n.Status); err != nil {
			b.replyf(ctx, n.Status, "@%s sorry, I couldn't set that reminder.", n.Status.Account.Acct)
		}
	case "help":
		var prompt *Status
		prompt, err = b.reply(ctx, n.Status, helpMenu, n.Status.Account.Acct)
		if err == nil && prompt != nil {
			if perr := b.store.AddHelpPrompt(prompt.ID); perr != nil {
				log.Printf("recording help prompt: %v", perr)
			}
		}
	case "version":
		err = b.replyf(ctx, n.Status, "@%s I'm jacques v%s.", n.Status.Account.Acct, botVersion())
	}
	if err != nil {
		log.Printf("%s failed: %v", cmd, err)
	}
}

const helpMenu = "@%s here's what I can do. Reply to this with the name of one and I'll explain how it works:\n\n" +
	"unroll, refresh, remind, birthday, forget, version\n\n" +
	"(or mention me with no command on a public post and I'll boost it)"

type helpTopic struct {
	aliases     []string
	explanation string
}

var helpTopics = []helpTopic{
	{[]string{"unroll"}, "unroll: reply \"unroll\" to any post inside a thread and I'll lay the whole thing out on a single page you can link and share."},
	{[]string{"refresh"}, "refresh: reply \"refresh\" on a thread that's already unrolled and I'll update its page with any new posts."},
	{[]string{"remind", "reminder"}, "remind: say \"remind me in 3 days\" (also hours, weeks, or \"tomorrow\") and I'll nudge you right here when the time comes."},
	{[]string{"birthday"}, "birthday: tell me \"birthday June 12\" and I'll wish you a happy birthday every year. Say \"birthday forget\" to drop it."},
	{[]string{"forget", "remember", "optout"}, "forget me: message me \"forget me\" and I'll stop auto-unrolling your threads. Say \"remember me\" to opt back in."},
	{[]string{"version"}, "version: reply \"version\" and I'll tell you which build of me is currently running."},
	{[]string{"boost", "reblog"}, "boost: mention me with no command on a public post and I'll boost it for you."},
}

func matchHelpTopic(content string) *helpTopic {
	fields := cleanFields(content)
	for i := range helpTopics {
		for _, alias := range helpTopics[i].aliases {
			for _, f := range fields {
				if f == alias {
					return &helpTopics[i]
				}
			}
		}
	}
	return nil
}

func (b *Bot) helpSelection(ctx context.Context, mention *Status) {
	acct := mention.Account.Acct
	topic := matchHelpTopic(mention.Content)
	var reply *Status
	var err error
	if topic == nil {
		reply, err = b.reply(ctx, mention, "@%s hmm, I don't know that one. Try: unroll, refresh, remind, birthday, forget, or version.", acct)
	} else {
		reply, err = b.reply(ctx, mention, "@%s %s", acct, topic.explanation)
	}
	if err != nil {
		log.Printf("answering help selection from @%s: %v", acct, err)
		return
	}
	if reply != nil {
		if err := b.store.AddHelpPrompt(reply.ID); err != nil {
			log.Printf("recording help prompt: %v", err)
		}
	}
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

var commands = []string{"unroll", "refresh", "remind", "help", "version"}

func cleanFields(content string) []string {
	text := html.UnescapeString(tagRe.ReplaceAllString(content, " "))
	var fields []string
	for _, f := range strings.Fields(text) {
		fields = append(fields, strings.ToLower(strings.Trim(f, `.,!?:;"'`)))
	}
	return fields
}

func parseCommand(content string) string {
	for _, word := range cleanFields(content) {
		for _, cmd := range commands {
			if word == cmd {
				return cmd
			}
		}
	}
	return ""
}

func hasWord(content, word string) bool {
	for _, f := range cleanFields(content) {
		if f == word {
			return true
		}
	}
	return false
}

func hasPhrase(content string, words ...string) bool {
	fields := cleanFields(content)
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

func (b *Bot) threadRoot(ctx context.Context, mention *Status) (*Status, error) {
	mentionCtx, err := b.client.Context(ctx, mention.ID)
	if err != nil {
		return nil, err
	}
	if len(mentionCtx.Ancestors) == 0 {
		return nil, nil
	}
	root := mentionCtx.Ancestors[0]
	return &root, nil
}

func (b *Bot) unroll(ctx context.Context, mention *Status) error {
	if mention.InReplyToID == nil {
		return b.replyf(ctx, mention, "@%s reply to a post inside a thread and I'll unroll it for you.", mention.Account.Acct)
	}

	root, err := b.threadRoot(ctx, mention)
	if err != nil {
		return err
	}
	if root == nil {
		return b.replyf(ctx, mention, "@%s I couldn't find the thread this belongs to.", mention.Account.Acct)
	}

	if root.Account.ID != mention.Account.ID {
		optedOut, err := b.store.IsOptedOut(root.Account.ID)
		if err != nil {
			return err
		}
		if optedOut || hasNoBot(root.Account.Note) {
			return b.replyf(ctx, mention, "@%s the author of this thread prefers not to have their threads unrolled, so I'll leave it be.", mention.Account.Acct)
		}
	}

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
	chain := buildChain(*root, rootCtx.Descendants)

	page := b.buildPage(*root, chain)
	if err := b.store.SaveUnroll(page, mention.Account.Acct); err != nil {
		return err
	}
	return b.replyf(ctx, mention, "@%s here you go, %d posts by @%s unrolled: %s",
		mention.Account.Acct, len(chain), root.Account.Acct, b.pageURL(root.ID))
}

func (b *Bot) refresh(ctx context.Context, mention *Status) error {
	if mention.InReplyToID == nil {
		return b.replyf(ctx, mention, "@%s reply to a post inside your thread and I'll refresh its unrolled page.", mention.Account.Acct)
	}
	root, err := b.threadRoot(ctx, mention)
	if err != nil {
		return err
	}
	if root == nil {
		return b.replyf(ctx, mention, "@%s I couldn't find the thread this belongs to.", mention.Account.Acct)
	}
	existing, err := b.store.GetUnroll(root.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return b.unroll(ctx, mention)
	}

	rootCtx, err := b.client.Context(ctx, root.ID)
	if err != nil {
		return err
	}
	chain := buildChain(*root, rootCtx.Descendants)
	page := b.buildPage(*root, chain)
	if err := b.store.RefreshUnroll(page); err != nil {
		return err
	}
	log.Printf("refreshed thread %s by @%s (%d -> %d posts)", root.ID, root.Account.Acct, existing.PostCount, len(chain))
	return b.replyf(ctx, mention, "@%s refreshed! The page now shows %d posts: %s",
		mention.Account.Acct, len(chain), b.pageURL(root.ID))
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

func (b *Bot) replyf(ctx context.Context, to *Status, format string, args ...any) error {
	_, err := b.reply(ctx, to, format, args...)
	return err
}

func (b *Bot) reply(ctx context.Context, to *Status, format string, args ...any) (*Status, error) {
	visibility := to.Visibility
	if visibility == "" {
		visibility = "direct"
	}
	return b.client.Reply(ctx, to, visibility, fmt.Sprintf(format, args...))
}

func (b *Bot) replyPublicf(ctx context.Context, to *Status, format string, args ...any) error {
	_, err := b.client.Reply(ctx, to, "public", fmt.Sprintf(format, args...))
	return err
}
