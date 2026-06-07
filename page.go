package main

import (
	"html"
	"strings"
	"unicode"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func plainText(s string) string {
	body := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(s), body)
	if err != nil {
		text := html.UnescapeString(tagRe.ReplaceAllString(s, " "))
		return strings.Join(strings.Fields(text), " ")
	}
	var b strings.Builder
	for _, n := range nodes {
		plainNode(n, &b)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func plainNode(n *xhtml.Node, b *strings.Builder) {
	if n.Type == xhtml.TextNode {
		b.WriteString(n.Data)
		return
	}
	if n.Type == xhtml.ElementNode {
		switch n.Data {
		case "p", "br", "li", "blockquote", "pre":
			b.WriteString(" ")
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		plainNode(c, b)
	}
}

func (p *PageData) Excerpt() string {
	if len(p.Posts) == 0 {
		return ""
	}
	const max = 200
	text := plainText(p.Posts[0].HTML)
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	cut := max
	for cut > 0 && !unicode.IsSpace(runes[cut]) {
		cut--
	}
	if cut == 0 {
		cut = max
	}
	return strings.TrimRight(string(runes[:cut]), " .,;:") + "…"
}

func (p *PageData) FirstImage() string {
	for _, post := range p.Posts {
		for _, m := range post.Media {
			if m.Type == "image" {
				return m.URL
			}
		}
	}
	return p.Author.Avatar
}

func (p *PageData) ReadingMinutes() int {
	words := 0
	for _, post := range p.Posts {
		words += len(strings.Fields(plainText(post.HTML)))
	}
	minutes := (words + 199) / 200
	if minutes < 1 {
		minutes = 1
	}
	return minutes
}
