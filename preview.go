package main

import "time"

func samplePage() *PageData {
	bot := &Bot{policy: newPolicy()}
	author := Account{
		ID:          "1",
		Acct:        "rocky",
		DisplayName: "Rocky Raccoon",
		Avatar:      "https://picsum.photos/seed/raccoon-avatar/200/200",
		URL:         "https://raccoonisland.social/@rocky",
	}
	at := func(minute int) time.Time {
		return time.Date(2026, 6, 1, 18, minute, 0, 0, time.UTC)
	}
	root := Status{
		ID:        "preview",
		CreatedAt: at(0),
		Account:   author,
		URL:       "https://raccoonisland.social/@rocky/1",
		Content:   `<p>I spent the last month rebuilding my home network from scratch. Here is everything I learned, the mistakes I made, and what I would do differently next time. A thread. 🧵</p><p><a href="https://raccoonisland.social/tags/homelab" class="mention hashtag" rel="tag">#<span>homelab</span></a> <a href="https://raccoonisland.social/tags/networking" class="mention hashtag" rel="tag">#<span>networking</span></a></p>`,
	}
	chain := []Status{
		root,
		{
			ID:        "p2",
			CreatedAt: at(4),
			Account:   author,
			URL:       "https://raccoonisland.social/@rocky/2",
			Content:   `<p>First, the router. I ditched the ISP box and went with OpenWRT on a cheap fanless x86 machine after reading this excellent guide: <a href="https://openwrt.org/docs/guide-user/installation/openwrt_x86" target="_blank" rel="nofollow noopener noreferrer"><span class="invisible">https://</span><span class="ellipsis">openwrt.org/docs/guide-user/inst</span><span class="invisible">allation/openwrt_x86</span></a></p><p>Total cost: 120€. It has been rock solid for three weeks straight, and idle power draw sits under 10W.</p>`,
			MediaAttachments: []MediaAttachment{
				{Type: "image", URL: "https://picsum.photos/seed/router-box/1200/800", Description: "A small fanless mini PC sitting on a wooden desk next to a patch cable"},
			},
		},
		{
			ID:        "p3",
			CreatedAt: at(11),
			Account:   author,
			URL:       "https://raccoonisland.social/@rocky/3",
			Content:   `<p>Huge thanks to <span class="h-card" translate="no"><a href="https://raccoonisland.social/@trash_panda" class="u-url mention">@<span>trash_panda</span></a></span> for talking me out of the managed switch I almost bought and into something half the price. Wiring the rack was honestly the most fun part of the whole project:</p>`,
			MediaAttachments: []MediaAttachment{
				{Type: "image", URL: "https://picsum.photos/seed/rack-front/1200/900", Description: "Front view of a small network rack with neatly routed orange patch cables"},
				{Type: "image", URL: "https://picsum.photos/seed/rack-back/1200/900", Description: "Back view of the same rack showing cable management bars"},
			},
		},
		{
			ID:        "p4",
			CreatedAt: at(19),
			Account:   author,
			URL:       "https://raccoonisland.social/@rocky/4",
			Content:   `<p>The biggest lesson: <strong>VLANs before hardware</strong>. Plan the segments on paper first — <em>then</em> buy what supports them. My layout ended up as <code>10.0.10.0/24</code> for trusted devices, <code>10.0.20.0/24</code> for IoT junk and <code>10.0.30.0/24</code> for guests.</p><blockquote><p>Every untagged port is a future security incident you scheduled for yourself.</p></blockquote><p>I have that taped above my desk now.</p>`,
		},
		{
			ID:        "p5",
			CreatedAt: at(27),
			Account:   author,
			URL:       "https://raccoonisland.social/@rocky/5",
			Content:   `<p>That's the thread! Full write-up with config files and the shopping list is on my blog: <a href="https://example.com/posts/home-network-rebuild" target="_blank" rel="nofollow noopener noreferrer"><span class="invisible">https://</span><span class="ellipsis">example.com/posts/home-network-r</span><span class="invisible">ebuild</span></a></p><p>Questions welcome, replies are open. <a href="https://raccoonisland.social/tags/selfhosting" class="mention hashtag" rel="tag">#<span>selfhosting</span></a></p>`,
			MediaAttachments: []MediaAttachment{
				{Type: "image", URL: "https://picsum.photos/seed/desk-setup/1200/700", Description: "A tidy desk with a laptop showing a network dashboard full of green checkmarks"},
			},
		},
	}
	return bot.buildPage(root, chain)
}
