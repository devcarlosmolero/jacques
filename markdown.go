package main

import (
	"fmt"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func renderMarkdown(p *PageData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s (@%s) — thread, unrolled\n\n", p.Author.DisplayName, p.Author.Acct)
	fmt.Fprintf(&b, "%d posts · %s · [view original thread](%s)\n\n", p.PostCount, p.StartedAt.Format("Jan 2, 2006"), p.RootURL)
	for i, post := range p.Posts {
		b.WriteString("---\n\n")
		if md := htmlToMarkdown(post.HTML); md != "" {
			b.WriteString(md + "\n\n")
		}
		for _, m := range post.Media {
			fmt.Fprintf(&b, "![%s](%s)\n\n", mdAltText(m.Description), m.URL)
		}
		fmt.Fprintf(&b, "*[%d/%d · %s](%s)*\n\n", i+1, len(p.Posts), post.CreatedAt.Format("Jan 2, 2006 15:04"), post.URL)
	}
	b.WriteString("---\n\nunrolled by [jacques](https://raccoonisland.social/@jacques)\n")
	return b.String()
}

func mdAltText(s string) string {
	s = strings.NewReplacer("[", "(", "]", ")", "\n", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

var blankLines = regexp.MustCompile(`\n{3,}`)

func htmlToMarkdown(fragment string) string {
	body := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(fragment), body)
	if err != nil {
		return plainText(fragment)
	}
	var b strings.Builder
	for _, n := range nodes {
		b.WriteString(nodeMD(n))
	}
	out := blankLines.ReplaceAllString(b.String(), "\n\n")
	return strings.TrimSpace(out)
}

func nodeMD(n *xhtml.Node) string {
	switch n.Type {
	case xhtml.TextNode:
		return n.Data
	case xhtml.ElementNode:
		switch n.Data {
		case "br":
			return "\n"
		case "p":
			return strings.TrimSpace(childrenMD(n)) + "\n\n"
		case "strong", "b":
			return "**" + childrenMD(n) + "**"
		case "em", "i":
			return "*" + childrenMD(n) + "*"
		case "del", "s":
			return "~~" + childrenMD(n) + "~~"
		case "code":
			return "`" + childrenMD(n) + "`"
		case "pre":
			return "```\n" + strings.TrimRight(textContent(n), "\n") + "\n```\n\n"
		case "blockquote":
			var out strings.Builder
			for _, line := range strings.Split(strings.TrimSpace(childrenMD(n)), "\n") {
				out.WriteString("> " + line + "\n")
			}
			return out.String() + "\n"
		case "a":
			href := attrValue(n, "href")
			text := strings.TrimSpace(childrenMD(n))
			if href == "" {
				return text
			}
			if text == "" {
				text = href
			}
			return "[" + text + "](" + href + ")"
		case "span":
			class := attrValue(n, "class")
			if strings.Contains(class, "invisible") {
				return ""
			}
			if strings.Contains(class, "ellipsis") {
				return childrenMD(n) + "…"
			}
			return childrenMD(n)
		case "ul", "ol":
			var out strings.Builder
			i := 1
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == xhtml.ElementNode && c.Data == "li" {
					marker := "- "
					if n.Data == "ol" {
						marker = fmt.Sprintf("%d. ", i)
						i++
					}
					out.WriteString(marker + strings.TrimSpace(childrenMD(c)) + "\n")
				}
			}
			return out.String() + "\n"
		default:
			return childrenMD(n)
		}
	default:
		return ""
	}
}

func childrenMD(n *xhtml.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(nodeMD(c))
	}
	return b.String()
}

func textContent(n *xhtml.Node) string {
	if n.Type == xhtml.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textContent(c))
	}
	return b.String()
}

func attrValue(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
