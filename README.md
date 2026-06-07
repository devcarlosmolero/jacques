# jacques

<img src="static/jacques.png" alt="jacques the raccoon" width="120" align="right">

Jacques, yes, *that* Jacques, the raccoon from Adventure Island, has hung up his adventuring gear and settled down on [raccoonisland.social](https://raccoonisland.social), where he now spends his days doing what raccoons do best: rummaging through threads and dragging the good stuff out into the open.

He's a multipurpose bot written in Go. He starts small, but he's a fast learner, and new commands will keep arriving as development advances. His current version is always listed on [his profile](https://raccoonisland.social/@jacques).

<a href="https://codeberg.org/devcarlosmolero/jacques" target="_blank" noopener noreferrer><img width="200px" src="https://codeberg.org/devcarlosmolero/jacques/raw/branch/master/mirror.svg"></a>

## What jacques can do today

### unroll

Reply `@jacques unroll` to any post inside a thread and jacques answers with a link to the whole thread laid out as a single page. No tapping through reply after reply, no losing your place halfway down: just the author's posts in order, with their links, mentions, hashtags and images, typeset for comfortable reading from top to bottom.

Threads are only ever unrolled once. If someone already asked for that thread, jacques hands you the existing link straight away. If the thread's author has opted out (see below), jacques tells you so and leaves the thread alone; only the author's own request overrides that.

Jacques always matches how you talked to him: ask publicly and he answers publicly, ask in a private mention and the answer goes to you and nobody else. This holds for every command, including errors.

### refresh

Added more posts to a thread after it was unrolled? Reply `@jacques refresh` anywhere in the thread and he'll re-read it and update the page in place. Same URL, so links that are already out there keep working. Only the thread's author can refresh.

### remind

Reply `@jacques remind me in 3 days` to any post and he'll come back and nudge you right there when the time is up. He understands minutes, hours, days, weeks and months (`in 2 hours`, `in a week`, `tomorrow`, `next month`), from one minute up to a year. The confirmation and the later nudge both use the same visibility as your request.

### birthday

Send jacques a private mention saying `birthday June 12` (or `birthday 12 June`, month names can be abbreviated to three letters) and every year on that day he'll post a public birthday wish for you. Born on February 29? He celebrates on the 28th when the year is short a day. Say `birthday forget` and he drops the date entirely.

### boost

Mention `@jacques` in any public or unlisted post, no command needed, and he'll boost it. A little raccoon megaphone for things you want amplified.

### help & version

A mention containing `help` gets you a rundown of everything above; `version` tells you which build of jacques is running.

### auto-unroll

Jacques also rummages on his own. He watches the federated timeline for authors replying to themselves, and when a self-thread reaches 5 posts and has been quiet for 15 minutes, he unrolls it and replies publicly with the link. One reply per thread, ever; at most 4 per hour; bots and conversations that merely contain self-replies are left alone. These announcements, the monthly report and birthday wishes are the only things jacques posts publicly on his own initiative; everything else follows the visibility of whoever talked to him.

Don't want him around? Put `#nobot` in your bio, or send him a private mention saying `forget me` and he'll drop what he'd gathered about your threads and never unroll you on his own again (`remember me` undoes it). Everything is tunable or can be switched off via the `JACQUES_AUTO_UNROLL*` environment variables in `main.go`.

### monthly rummage report

Once a month, jacques posts a little public recap on his own: how many threads he unrolled the previous month, how many posts that added up to, and his all-time count. Switch it off with `JACQUES_MONTHLY_STATS=off`.

## The unrolled pages

Every page comes with the right Open Graph tags (title, excerpt, first image of the thread), so an unroll link shared on Mastodon, or anywhere else, gets a proper preview card. The header shows the post count and an estimated reading time, every post has its own anchor (`#post-3`) for sharing a specific spot in a thread, and a `markdown` link in the footer (or appending `.md` to any page URL) gives you the whole thread as plain markdown, handy for archiving. There's also a `/healthz` endpoint returning status and version for uptime monitoring.

## How he does it

Jacques polls his mentions and reacts. For an unroll, he fetches the mention's context, takes the first ancestor as the thread root, then repeatedly looks for the earliest reply the root author wrote to their own previous post. Other people replying in between never break the chain; the walk ends only when the author stops replying to themselves. The result is stored in SQLite keyed by the root status id, which is what makes repeat requests instant.

Mastodon hands over each post as ready-made HTML, and jacques deliberately doesn't rewrite it. He sanitizes it with bluemonday's UGC policy, keeping paragraphs, line breaks and links while stripping anything dangerous, and additionally allows the `class` attribute only for the handful of values Mastodon itself emits (`mention`, `hashtag`, `ellipsis`, `invisible`, `u-url`, `h-card`). Two tiny CSS rules reproduce Mastodon's link shortening on the unrolled page, and Tailwind's typography plugin handles the rest of the styling inside a `prose` container.

All links on an unrolled page are canonical: the author's name, every timestamp and the "view original thread" link point to the author's home instance, wherever that is.

## Versioning

Jacques follows [semver](https://semver.org). The current version lives in `version.json`, is baked into the binary at build time, and CI keeps the **Version** field on his Mastodon profile in sync. When a deploy ships a new version, the profile updates automatically.

Want to see what an unrolled thread looks like? Run `go run . -preview` and open `http://localhost:8080/t/preview` for a sample thread rendered exactly the way jacques serves them.
