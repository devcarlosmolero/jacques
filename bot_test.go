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

func TestParseCommand(t *testing.T) {
	cases := map[string]string{
		`<p><span class="h-card"><a href="https://example.com/@jacques">@jacques</a></span> unroll</p>`: "unroll",
		`<p><a href="https://example.com/@jacques">@jacques</a> UNROLL!</p>`:                            "unroll",
		`<p>@jacques please unroll this</p>`:                                                            "unroll",
		`<p>@jacques refresh</p>`:                                                                       "refresh",
		`<p>@jacques Help</p>`:                                                                          "help",
		`<p>@jacques version?</p>`:                                                                      "version",
		`<p>@jacques what do you think?</p>`:                                                            "",
		`<p>talking about unrolling things</p>`:                                                         "",
	}
	for content, want := range cases {
		if got := parseCommand(content); got != want {
			t.Errorf("parseCommand(%q) = %q, want %q", content, got, want)
		}
	}
}

func TestParseRemindDuration(t *testing.T) {
	cases := map[string]struct {
		d  time.Duration
		ok bool
	}{
		`<p>@jacques remind me in 3 days</p>`:       {3 * 24 * time.Hour, true},
		`<p>@jacques remind me in 2 hours!</p>`:     {2 * time.Hour, true},
		`<p>@jacques remind me in 30 minutes</p>`:   {30 * time.Minute, true},
		`<p>@jacques remind me in 30 mins</p>`:      {30 * time.Minute, true},
		`<p>@jacques remind me tomorrow</p>`:        {24 * time.Hour, true},
		`<p>@jacques remind me next week</p>`:       {7 * 24 * time.Hour, true},
		`<p>@jacques remind me in a month</p>`:      {30 * 24 * time.Hour, true},
		`<p>@jacques remind me in an hour</p>`:      {time.Hour, true},
		`<p>@jacques Remind me in 1 week.</p>`:      {7 * 24 * time.Hour, true},
		`<p>@jacques remind me</p>`:                 {0, false},
		`<p>@jacques remind me someday</p>`:         {0, false},
		`<p>@jacques remind me in 0 days</p>`:       {0, false},
		`<p>@jacques remind me in 900 months</p>`:   {0, false},
		`<p>@jacques remind me in 10 seconds</p>`:   {0, false},
		`<p>@jacques what do you think?</p>`:        {0, false},
		`<p>@jacques remind me in 2 days to vote`:   {2 * 24 * time.Hour, true},
		`<p>@jacques please remind me in 1 day</p>`: {24 * time.Hour, true},
	}
	for content, want := range cases {
		d, ok := parseRemindDuration(content)
		if d != want.d || ok != want.ok {
			t.Errorf("parseRemindDuration(%q) = (%v, %v), want (%v, %v)", content, d, ok, want.d, want.ok)
		}
	}
}

func TestReminderLifecycle(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := store.AddReminder("s1", "alice", "public", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.AddReminder("s2", "bob", "direct", now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	due, err := store.DueReminders(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("expected nothing due yet, got %+v", due)
	}

	due, err = store.DueReminders(now.Add(2 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].StatusID != "s1" || due[0].Acct != "alice" || due[0].Visibility != "public" {
		t.Fatalf("expected alice's public reminder due, got %+v", due)
	}

	if err := store.DeleteReminder(due[0].ID); err != nil {
		t.Fatal(err)
	}
	due, err = store.DueReminders(now.Add(2 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("expected delivered reminder to be gone, got %+v", due)
	}

	due, err = store.DueReminders(now.Add(72 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].StatusID != "s2" {
		t.Fatalf("expected bob's reminder due later, got %+v", due)
	}
}

func TestParseBirthday(t *testing.T) {
	cases := map[string]struct {
		month time.Month
		day   int
		ok    bool
	}{
		`<p>@jacques birthday June 12</p>`:        {time.June, 12, true},
		`<p>@jacques birthday 12 June</p>`:        {time.June, 12, true},
		`<p>@jacques birthday june 12th!</p>`:     {time.June, 12, true},
		`<p>@jacques my birthday is on Dec 1</p>`: {time.December, 1, true},
		`<p>@jacques birthday February 29</p>`:    {time.February, 29, true},
		`<p>@jacques birthday February 30</p>`:    {0, 0, false},
		`<p>@jacques birthday June</p>`:           {0, 0, false},
		`<p>@jacques birthday 12</p>`:             {0, 0, false},
		`<p>@jacques birthday</p>`:                {0, 0, false},
		`<p>@jacques unroll</p>`:                  {0, 0, false},
	}
	for content, want := range cases {
		month, day, ok := parseBirthday(content)
		if month != want.month || day != want.day || ok != want.ok {
			t.Errorf("parseBirthday(%q) = (%v, %d, %v), want (%v, %d, %v)", content, month, day, ok, want.month, want.day, want.ok)
		}
	}
}

func TestBirthdayLifecycle(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SetBirthday("acc1", "alice", time.June, 12); err != nil {
		t.Fatal(err)
	}
	if err := store.SetBirthday("acc2", "bob", time.June, 12); err != nil {
		t.Fatal(err)
	}
	if err := store.SetBirthday("acc3", "carol", time.December, 1); err != nil {
		t.Fatal(err)
	}

	accts, err := store.BirthdaysOn(time.June, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(accts) != 2 || accts[0] != "alice" || accts[1] != "bob" {
		t.Fatalf("BirthdaysOn(June, 12) = %v, want [alice bob]", accts)
	}

	if err := store.SetBirthday("acc1", "alice", time.July, 1); err != nil {
		t.Fatal(err)
	}
	accts, err = store.BirthdaysOn(time.June, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(accts) != 1 || accts[0] != "bob" {
		t.Fatalf("expected alice's birthday to move, got %v", accts)
	}

	if err := store.RemoveBirthday("acc2"); err != nil {
		t.Fatal(err)
	}
	accts, err = store.BirthdaysOn(time.June, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(accts) != 0 {
		t.Fatalf("expected no birthdays left on June 12, got %v", accts)
	}
}

func TestRefreshUnrollUpdatesInPlace(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	page := samplePage()
	if err := store.SaveUnroll(page, "rocky"); err != nil {
		t.Fatal(err)
	}
	page.Posts = page.Posts[:3]
	page.PostCount = 3
	if err := store.RefreshUnroll(page); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetUnroll(page.RootID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.PostCount != 3 || len(got.Posts) != 3 {
		t.Fatalf("expected refreshed page with 3 posts, got %+v", got)
	}
}

func TestUnrollStats(t *testing.T) {
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	page := samplePage()
	if err := store.SaveUnroll(page, "rocky"); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	threads, posts, err := store.UnrollStats(now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if threads != 1 || posts != page.PostCount {
		t.Fatalf("UnrollStats = (%d, %d), want (1, %d)", threads, posts, page.PostCount)
	}

	threads, posts, err = store.UnrollStats(now.Add(-2*time.Hour), now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if threads != 0 || posts != 0 {
		t.Fatalf("UnrollStats outside window = (%d, %d), want (0, 0)", threads, posts)
	}

	total, err := store.TotalUnrolls()
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("TotalUnrolls = %d, want 1", total)
	}
}
