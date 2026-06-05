package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPageTemplateRendersSampleThread(t *testing.T) {
	var buf bytes.Buffer
	if err := pageTemplate.Execute(&buf, samplePage()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Rocky Raccoon",
		"@rocky",
		"5 posts",
		`class="invisible"`,
		`class="ellipsis"`,
		"openwrt.org/docs/guide-user/inst",
		"trash_panda",
		"<strong>VLANs before hardware</strong>",
		"<blockquote>",
		"<code>10.0.10.0/24</code>",
		"picsum.photos/seed/rack-front",
		"Front view of a small network rack",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page is missing %q", want)
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
