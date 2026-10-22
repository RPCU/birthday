# 🎂 Birthday

A little countdown to someone's birthday, hiding a surprise: an I.O.U. card
(*"I owe you a birthday drink"* by default). Before the birthday the page
only teases it: a rattling gift box with *"A little something is waiting for
you"* (`BIRTHDAY_TEASER`) that unlocks when the countdown ends. The server
leaves the card, its line and its signature out of the page, the link preview
and the JSON feed until then. When the countdown ends the
page switches to a *Happy birthday* greeting with confetti and the card
appears, its line unveiling. Click the champagne glass for a toast. The card
stays for `BIRTHDAY_OPEN_DAYS` (15) days after the birthday while the
countdown moves on to next year, then hides until the next one.

It's one static Go binary (standard library only, with the template, assets
and time zone database embedded), configured entirely through environment
variables. The optional [chat friend](#chat-friend) also needs Claude Code,
which the image ships.

## Configuration

| Variable              | Required | Default                      | Description                                                            |
| --------------------- | -------- | ---------------------------- | ---------------------------------------------------------------------- |
| `BIRTHDAY_NAME`       | no       | `friend`                     | Who the countdown is for (max 64 characters).                          |
| `BIRTHDAY_DATE`       | no       | `01-01`                      | `MM-DD`, or `YYYY-MM-DD` to also show the age they are turning.        |
| `BIRTHDAY_MESSAGE`    | no       | `I owe you a birthday drink` | The line on the I.O.U. card, hidden until the birthday (max 280 characters). |
| `BIRTHDAY_FROM`       | no       |                              | Signature under the line, hidden with it.                              |
| `BIRTHDAY_SECRET`     | no       |                              | A hidden extra revealed with the I.O.U. (a gift card code or link), blurred until tapped, then tap to copy. Meant to come from a secret store (max 500 characters). |
| `BIRTHDAY_SECRET_PIN` | no       |                              | Optional PIN (4–12 characters) required to reveal the secret. When set, the page never contains the secret: `POST /api/reveal` checks the PIN server-side (constant time, ~5 tries/minute, same-origin only) and returns it. |
| `BIRTHDAY_SECRET_INTRO` | no     | `And a little extra, just for you` | The line announcing the secret on the card. |
| `BIRTHDAY_TEASER`     | no       | `A little something is waiting for you` | Teaser shown in the card's place before it unlocks (max 140 characters). |
| `BIRTHDAY_OPEN_DAYS`  | no       | `15`                         | Days the I.O.U. stays visible after the birthday (0–365).              |
| `BIRTHDAY_TIMEZONE`   | no       | `UTC`                        | IANA zone used for midnight, e.g. `Europe/Paris`.                      |
| `BIRTHDAY_HALLOWEEN`  | no       | `true`                       | Subtle seasonal touches: a crescent moon, two bats and a corner cobweb. |
| `BIRTHDAY_CHAT`       | no       | `false`                      | Adds a chat with a cheerful friend to the opened card, above the secret (see [Chat friend](#chat-friend)). |
| `BIRTHDAY_CHAT_NAME`  | no       | `Sunny`                      | The friend's name (max 64 characters).                                 |
| `BIRTHDAY_CHAT_PROMPT` | no      |                              | More instructions on how the friend acts, added to the built-in cheerful persona (max 8000 characters). |
| `BIRTHDAY_CHAT_FALLBACK` | no    | six cheerful birthday lines  | What the friend says, one line per reply in turn, when Claude can't answer; `{name}` becomes `BIRTHDAY_NAME` (one per line, max 50 lines of 280 characters). |
| `BIRTHDAY_CHAT_HISTORY` | no     | `true`                       | Keep today's conversation (see [Chat friend](#chat-friend)).            |
| `BIRTHDAY_CHAT_NOTES`   | no     | `true`                       | Keep their answers for you (see [Chat friend](#chat-friend)).           |
| `BIRTHDAY_CHAT_BEFORE_PIN` | no  | `5`                          | Messages to the friend, after the PIN, before the secret (0 to 50).     |
| `BIRTHDAY_CHAT_COUNT_RESET` | no | `true`                       | Start those messages over on every visit (`false`: once is enough).     |
| `BIRTHDAY_CHAT_MODEL` | no       | `sonnet`                     | Claude model (`sonnet`, `opus`, `haiku` or a full model ID).           |
| `BIRTHDAY_CHAT_COMMAND` | no     | `claude`                     | The Claude Code CLI.                                                   |
| `BIRTHDAY_HOST`       | no       | all interfaces               | Listen address.                                                        |
| `BIRTHDAY_PORT`       | no       | `8080`                       | Listen port.                                                           |

Every setting is optional. Without `BIRTHDAY_DATE` the countdown runs to
January 1 (a fixed day, so a missing date never opens the surprise early)
and the server logs a warning. A 29 February birthday is celebrated on 28
February in non-leap years. An invalid value stops the server at startup and
lists every problem.

Over the last ten days the countdown page dresses up, a little more each
day: a bunting garland fills in from the middle (a pennant a day), gold
sparkles twinkle (one more a day) and, from five days out, a few confetti
flecks drift by. All of it stays faint and behind the countdown, and
`prefers-reduced-motion` stills it.

## Chat friend

With `BIRTHDAY_CHAT=true`, the opened card holds a little chat with
`BIRTHDAY_CHAT_NAME`, a warm, cheerful friend who celebrates the birthday
person, played by Claude through Claude Code on a Claude login (no API key).
It's locked like the secret: it's not in the page, and `POST /api/chat`
answers 404, until the countdown ends, and with `BIRTHDAY_SECRET_PIN` set it
stays locked until that PIN is entered (one PIN unlocks both).

`BIRTHDAY_CHAT_PROMPT` teaches the friend more: in-jokes, what to call them,
what to bring up or avoid. The built-in persona knows the name, the date, the
age and the card's line, never the secret. It's warm but classy, and gently
gets to know their tastes: one light question at a time (favourite flowers,
colours, scents, food and drinks, music, films, books, style, dream trips,
their perfect day), never prying into relationships, health, money
or anything private, and dropping whatever they dodge. It stays in character
but won't deny being an AI if asked sincerely, nor that the page's maker can
read the chat if asked.

**The PIN, then a chat, then the secret.** With the chat, a secret and its
PIN, the PIN opens the chat first, and the secret waits until they've sent
the friend `BIRTHDAY_CHAT_BEFORE_PIN` messages (5; `0` shows it with the PIN
right away). Meanwhile a pumpkin ripens with each message, with a dot per
message and a hint at each step; once it's ripe, the secret shows by itself
(the same PIN). `POST /api/reveal` answers 409 until then (without costing a
PIN try). The count starts over every time the PIN opens the chat (so on
every visit) and every time the secret is revealed; with `BIRTHDAY_CHAT_COUNT_RESET=false` it never resets, and
once reached the PIN reveals the secret right away. It lives next to the
login (`$CLAUDE_CONFIG_DIR/birthday-talked`, shared by the replicas).

**Their answers.** Every message they send is added, after the friend's line
it answers, to `$CLAUDE_CONFIG_DIR/birthday-answers.md`, next to the login.
Unlike the day's conversation it's never cleared, and the page never shows
it. Read it with:

```sh
kubectl exec <pod> -- cat /home/birthday/.claude/birthday-answers.md
```

`BIRTHDAY_CHAT_NOTES=false` keeps no notes.

The friend is a blank slate, cut off from the account it runs on: every
message runs a fresh `claude --print` in an empty directory with no tools, no
MCP servers or claude.ai connectors, no plugins, skills, hooks, settings,
`CLAUDE.md` or memory, no saved session, its own system prompt instead of
Claude Code's, and none of the `BIRTHDAY_*` variables. The page sends the
conversation with each message (its last 50), and the server keeps today's
in `$CLAUDE_CONFIG_DIR/birthday-chat/<date>.json`, next to the login, so it
comes back on reload, on another device and on every replica sharing the
volume. Only today's file (in `BIRTHDAY_TIMEZONE`) is read and the older ones
are deleted, so it starts over every midnight. Reading it back is locked like
the chat (open card, then the PIN). `BIRTHDAY_CHAT_HISTORY=false` keeps
nothing. Replies stream in, 6 a minute (12 in a burst). Claude Code runs one at a
time, taking turns on a lock file next to the login
(`$CLAUDE_CONFIG_DIR/birthday-claude.lock`), so replicas sharing the volume
never renew the login at once (two renewals at once can sign the friend out);
a second message waits for the first reply.

**Signing in.** The image has Claude Code and a shell. Its login lives in
`CLAUDE_CONFIG_DIR` (`/home/birthday/.claude`), apart from anything else:

```sh
kubectl exec -it <pod> -- claude auth login
```

Mount a volume on `/home/birthday/.claude` to keep that login (and today's
conversation) across restarts (hestia's `persistent-volume` trait, `claude` instance, does it per
environment); without one it lasts as long as the container. Either way it
covers one replica. For
something that survives restarts, set `CLAUDE_CODE_OAUTH_TOKEN` (from
`claude setup-token`) from a secret instead. Until it's signed in (or when
Claude Code is missing or out of usage), the friend still answers, with the
`BIRTHDAY_CHAT_FALLBACK` lines in turn after a short "typing" pause, and the
server logs a warning; Claude is tried again a minute later. Each reply runs a Claude Code
process of about 250 MiB, two at most at once: give the container ~640 MiB
and 2 CPUs (on 1 CPU it starts too slowly and gateways time out).

## Endpoints

- `GET /`: the countdown page.
- `GET /api/countdown`: the same state as JSON (`start`, `end`, `today`,
  `turning`, `now`, `open`, `openUntil`, …). `message` is only present while
  `open` is true.
- `POST /api/reveal` (`{"pin": "…"}`): returns the PIN-guarded secret while the card is open; 403 on a wrong PIN, 429 when throttled, 404 when there's nothing to reveal.
- `POST /api/chat` (`{"pin": "…", "messages": [{"from": "me"|"friend", "text": "…"}]}`): the friend's reply to the last message, streamed as JSON lines (`{"text"}`…, then `{"done": true}` or `{"error"}`), while the card is open and `BIRTHDAY_CHAT` is on; without messages it checks the PIN and returns today's conversation (`{"messages": […]}`). Wrong PINs count against `/api/reveal`'s budget.
- `GET /healthz`: liveness/readiness probe.

## Development

```sh
devenv shell                      # Go, gofumpt, golines, air; sample BIRTHDAY_* set
go run . serve                    # http://127.0.0.1:8080
BIRTHDAY_DATE=$(date +%m-%d) go run . serve   # preview the birthday itself
```

Checks (CI runs the same):

```sh
go vet ./... && go test ./... && gofumpt -d . && golines --max-len=160 . --dry-run
nix-build nix/oci.nix --argstr doCheck false   # when go.mod or nix/ change
```

## Deployment

The flow is the same as chihiro's:

1. A push to `main` runs `.github/workflows/image.yaml`, which pushes
   `zot.rpcu.io/public/birthday:<8-char SHA>` (or `:<tag>` for tags).
2. The GitHub webhook triggers OpenChoreo's `prebuilt-image` workflow (hestia,
   `openchoreo/namespaces/rpcu/projects/testing/birthday.yaml`), which waits
   for that tag and publishes the workload described by `workload.yaml`
   (one HTTP endpoint on 8080).
3. The name, date, line and hostname are set per environment in hestia's
   ReleaseBindings.
