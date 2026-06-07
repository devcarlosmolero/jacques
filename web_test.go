package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPageTemplateRendersSampleThread(t *testing.T) {
	var buf bytes.Buffer
	if err := pageTemplate.Execute(&buf, pageView{PageData: samplePage(), BaseURL: "https://jacques.example"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Rocky Raccoon",
		"@rocky",
		"5 posts",
		"min read",
		`class="invisible"`,
		`class="ellipsis"`,
		"openwrt.org/docs/guide-user/inst",
		"trash_panda",
		"<strong>VLANs before hardware</strong>",
		"<blockquote>",
		"<code>10.0.10.0/24</code>",
		"picsum.photos/seed/rack-front",
		"Front view of a small network rack",
		`property="og:title"`,
		`property="og:image" content="https://picsum.photos/seed/router-box/1200/800"`,
		`property="og:url" content="https://jacques.example/t/preview"`,
		`rel="canonical" href="https://jacques.example/t/preview"`,
		`id="post-1"`,
		`href="#post-5"`,
		`href="https://jacques.example/t/preview.md"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}
}

func TestPageMetadataHelpers(t *testing.T) {
	page := samplePage()
	if got := page.FirstImage(); got != "https://picsum.photos/seed/router-box/1200/800" {
		t.Errorf("FirstImage = %q", got)
	}
	excerpt := page.Excerpt()
	if !strings.HasPrefix(excerpt, "I spent the last month rebuilding my home network") {
		t.Errorf("Excerpt = %q", excerpt)
	}
	if len([]rune(excerpt)) > 201 {
		t.Errorf("Excerpt too long: %d runes", len([]rune(excerpt)))
	}
	if got := page.ReadingMinutes(); got < 1 || got > 3 {
		t.Errorf("ReadingMinutes = %d, want a small positive number", got)
	}
}

func TestRenderMarkdown(t *testing.T) {
	out := renderMarkdown(samplePage())
	for _, want := range []string{
		"# Rocky Raccoon (@rocky) — thread, unrolled",
		"5 posts",
		"[view original thread](https://raccoonisland.social/@rocky/1)",
		"**VLANs before hardware**",
		"> Every untagged port is a future security incident",
		"`10.0.10.0/24`",
		"[openwrt.org/docs/guide-user/inst…](https://openwrt.org/docs/guide-user/installation/openwrt_x86)",
		"[@trash_panda](https://raccoonisland.social/@trash_panda)",
		"![A small fanless mini PC sitting on a wooden desk next to a patch cable](https://picsum.photos/seed/router-box/1200/800)",
		"[1/5",
		"unrolled by [jacques]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown is missing %q", want)
		}
	}
	for _, banned := range []string{"<p>", "</a>", "<span", "https://https://"} {
		if strings.Contains(out, banned) {
			t.Errorf("markdown still contains %q", banned)
		}
	}
}

func TestPolicyStripsDangerousHTML(t *testing.T) {
	p := newPolicy()
	in := `<p onclick="evil()">hello <script>alert(1)</script><a href="javascript:evil()">click</a><img src="x" onerror="evil()"><span class="bg-red-500">styled</span></p>`
	out := p.Sanitize(in)
	for _, banned := range []string{"<script", "onclick", "onerror", "javascript:", "bg-red-500"} {
		if strings.Contains(out, banned) {
			t.Errorf("sanitized output still contains %q: %s", banned, out)
		}
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("sanitized output lost legitimate text: %s", out)
	}
}

func TestPolicyKeepsMastodonMarkup(t *testing.T) {
	p := newPolicy()
	in := `<p>see <a href="https://example.com/x" target="_blank" rel="nofollow noopener noreferrer"><span class="invisible">https://</span><span class="ellipsis">example.com/x</span></a> and <span class="h-card"><a href="https://example.com/@bob" class="u-url mention">@<span>bob</span></a></span></p>`
	out := p.Sanitize(in)
	for _, want := range []string{`class="invisible"`, `class="ellipsis"`, `class="u-url mention"`, "@<span>bob</span>", `href="https://example.com/x"`} {
		if !strings.Contains(out, want) {
			t.Errorf("sanitized output is missing %q: %s", want, out)
		}
	}
}
