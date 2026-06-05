# jacques

<img src="static/jacques.png" alt="jacques the raccoon" width="120" align="right">

Jacques, yes, *that* Jacques, the raccoon from Adventure Island, has hung up his adventuring gear and settled down on [raccoonisland.social](https://raccoonisland.social), where he now spends his days doing what raccoons do best: rummaging through threads and dragging the good stuff out into the open.

He's a multipurpose bot written in Go. He starts small, but he's a fast learner, and new commands will keep arriving as development advances. His current version is always listed on [his profile](https://raccoonisland.social/@jacques).

## What jacques can do today

### unroll

Reply `@jacques unroll` to any post inside a thread and jacques answers with a link to the whole thread laid out as a single page. No tapping through reply after reply, no losing your place halfway down: just the author's posts in order, with their links, mentions, hashtags and images, typeset for comfortable reading from top to bottom.

Threads are only ever unrolled once. If someone already asked for that thread, jacques hands you the existing link straight away.

### boost

Mention `@jacques` in any public or unlisted post, no command needed, and he'll boost it. A little raccoon megaphone for things you want amplified.

## How he does it

Jacques polls his mentions and reacts. For an unroll, he fetches the mention's context, takes the first ancestor as the thread root, then repeatedly looks for the earliest reply the root author wrote to their own previous post. Other people replying in between never break the chain; the walk ends only when the author stops replying to themselves. The result is stored in SQLite keyed by the root status id, which is what makes repeat requests instant.

Mastodon hands over each post as ready-made HTML, and jacques deliberately doesn't rewrite it. He sanitizes it with bluemonday's UGC policy, keeping paragraphs, line breaks and links while stripping anything dangerous, and additionally allows the `class` attribute only for the handful of values Mastodon itself emits (`mention`, `hashtag`, `ellipsis`, `invisible`, `u-url`, `h-card`). Two tiny CSS rules reproduce Mastodon's link shortening on the unrolled page, and Tailwind's typography plugin handles the rest of the styling inside a `prose` container.

All links on an unrolled page are canonical: the author's name, every timestamp and the "view original thread" link point to the author's home instance, wherever that is.

## Versioning

Jacques follows [semver](https://semver.org). The current version lives in `version.json`, is baked into the binary at build time, and CI keeps the **Version** field on his Mastodon profile in sync. When a deploy ships a new version, the profile updates automatically.

Want to see what an unrolled thread looks like? Run `go run . -preview` and open `http://localhost:8080/t/preview` for a sample thread rendered exactly the way jacques serves them.
