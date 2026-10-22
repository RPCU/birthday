# AGENTS.md

Guidance for AI agents and contributors working in this repository.

## Golden rules

- **NEVER commit without explicit user confirmation.** Do not run `git commit`,
  `git push`, `git tag`, or any history-rewriting command unless the user has
  clearly asked for it in that turn.
- Match the commit style of the sibling repos (gitmoji): `feat ✨:`,
  `fix 🐛 (scope):`, `refactor 🎨:`.

## Project overview

Birthday is a one-page Go web app: a countdown to `BIRTHDAY_DATE` for
`BIRTHDAY_NAME`, an I.O.U. card showing `BIRTHDAY_MESSAGE` (default
"I owe you a birthday drink"), and a celebration state on the day itself.
There is no auth. Everything is read from the environment; the only thing
written is the chat friend's conversation of the day (`history.go`) and the
birthday person's answers (`notes.go`).

Module path: `github.com/Banh-Canh/birthday`.

- Entry point: `main.go` → `cmd/` (`serve` default, `version`; stdlib only).
- Env config + validation: `internal/config/`.
- Date math (next occurrence, 29 Feb, DST-safe day bounds, age): `internal/countdown/`.
- HTTP routes, security headers, logging: `internal/server/`.
- Template + static assets, embedded with `embed`: `web/`.
- Nix build (binary, OCI image): `nix/`.

## Build / test / verify

```sh
go build ./... && go vet ./... && go test ./...
gofumpt -d .                         # must print nothing
golines --max-len=160 . --dry-run    # must print nothing
nix-build nix/oci.nix --argstr doCheck false   # after go.mod / nix/ changes
```

Use `devenv shell -- <cmd>`, which provides Go, gofumpt, golines and air.

## Conventions

- Standard library only (the chat shells out to the `claude` CLI, which the
  images ship). Adding a dependency means `vendorHash` in
  `nix/binaries.nix` stops being `null` and has to be computed.
- Logging is `log/slog` (JSON) only. Hestia's `default-error-rate` alert fires
  on **any `ERROR` log line**, so log at ERROR only for real failures.
- New settings are `BIRTHDAY_*` env vars, validated in `config.Load`, which
  reports every invalid setting at once. Every setting is optional with a
  default (hestia leaves some unset in production/public); keep it that way.
  The default date is a fixed January 1, never "today", so a missing date
  can't open the surprise early. Document them in README.md and wire
  them into hestia's ReleaseBindings.
- The page has a strict CSP: no inline scripts or styles, and only Google
  Fonts is allowed off-origin. Pass data to JS through `data-*` attributes.
- The whole I.O.U. card is a surprise: `moment()` only fills `Message` while
  `Birthday.Reveal` says it's open (the birthday plus `BIRTHDAY_OPEN_DAYS`,
  default 15), and the template renders the card, its signature and the
  `og:description` only then. Never send any of it to the client earlier, not
  in HTML, JSON, meta tags or `data-*` attributes; tests assert this.
- `BIRTHDAY_SECRET` (e.g. a gift card) is revealed with the card only, and
  only in the page: never put it in the JSON feed, meta tags or logs.
  With `BIRTHDAY_SECRET_PIN` it isn't even in the page: only `POST /api/reveal`
  returns it, after a constant-time PIN check behind a global token bucket
  (`reveal.go`) and `http.CrossOriginProtection`. Never log the PIN; log wrong
  guesses at Warn, not Error (hestia alerts on ERROR lines).
- The chat friend (`BIRTHDAY_CHAT`, `chat.go`) is locked exactly like the
  secret: rendered and served (`POST /api/chat`) only while open, and behind
  `BIRTHDAY_SECRET_PIN` when set, with wrong PINs draining the same
  `pinTries` bucket. Keep Claude Code isolated (`claudeArgs`/`claudeEnv`):
  no tools, MCP servers, connectors, plugins, settings, memory or saved
  sessions, an empty working directory, and never `BIRTHDAY_*` in its
  environment. Never put the secret or the PIN in its prompt. Claude failures
  (not signed in, missing, limits) log at Warn, and the friend answers with
  the canned `BIRTHDAY_CHAT_FALLBACK` lines instead for a minute (`awayFor`).
  Today's conversation is saved next to the login
  (`$CLAUDE_CONFIG_DIR/birthday-chat/<date>.json`, atomic writes: replicas
  share the volume), read back only through the PIN-checked
  `POST /api/chat` with no messages, and every other day's file is deleted.
  Claude Code runs take turns (`turn.go`: an in-process slot plus a flock on
  `$CLAUDE_CONFIG_DIR/birthday-claude.lock`, shared by replicas on the
  volume): concurrent OAuth renewals revoke each other.
  Each message the birthday person sends is appended, after the friend's
  line before it, to `$CLAUDE_CONFIG_DIR/birthday-answers.md` (`notes.go`)
  for the page's owner: never cleared, never served. The persona gets to
  know them without prying (one light question at a time, nothing private)
  and admits the owner can read the chat if asked; keep it that way.
- Before it unlocks, a teaser (`BIRTHDAY_TEASER`, gift box) stands in for
  the card. It must stay generic: never derive it from the message or the
  signature.
- CSS/JS URLs carry `?v=<content hash>` (`fingerprint` in the server) so a
  deploy never pairs new HTML with stale cached assets. Keep any new asset
  linked from the template in that fingerprint.
- All dates come from the server (`data-now` corrects client clock skew).
  The JS only reloads once `/api/countdown` agrees the state changed.
- The image's default command must start the server (`serve`): OpenChoreo
  cannot set command/args. Both images (Dockerfile, `nix/oci.nix`) ship Claude
  Code, a shell for `claude auth login`, and a writable `/home/birthday`
  with `CLAUDE_CONFIG_DIR=/home/birthday/.claude`; keep them in sync.
- `workload.yaml` (repo root) is the OpenChoreo workload descriptor: it
  declares `endpoint-1`, HTTP on 8080, which hestia's `http-route` trait and
  the web-application ComponentType rely on. Keep its port in sync with
  `BIRTHDAY_PORT`, and don't delete it, or deployments stop rendering.
- Respect `prefers-reduced-motion` and keep both color schemes working when
  touching the CSS. The Halloween touches must stay faint and classy.

## CI / CD

- **ci.yaml**: vet, race tests, build.
- **linting.yaml**: gofumpt + golines (160).
- **image.yaml**: pushes `zot.rpcu.io/public/birthday:<8-char SHA>` (plus
  `:<tag>`). Needs the `REGISTRY_USER` / `REGISTRY_PASSWORD` repo secrets.
  OpenChoreo's `prebuilt-image` workflow (hestia) waits for that exact tag, so
  keep the tag scheme in sync.
- **nix-build.yaml**: builds `nix/oci.nix` when go.mod / nix change.
