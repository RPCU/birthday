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
There is no auth and no storage. Everything is read from the environment.

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

- Standard library only. Adding a dependency means `vendorHash` in
  `nix/binaries.nix` stops being `null` and has to be computed.
- Logging is `log/slog` (JSON) only. Hestia's `default-error-rate` alert fires
  on **any `ERROR` log line**, so log at ERROR only for real failures.
- New settings are `BIRTHDAY_*` env vars, validated in `config.Load`, which
  reports every invalid setting at once. Document them in README.md and wire
  them into hestia's ReleaseBindings.
- The page has a strict CSP: no inline scripts or styles, and only Google
  Fonts is allowed off-origin. Pass data to JS through `data-*` attributes.
- All dates come from the server (`data-now` corrects client clock skew).
  The JS only reloads once `/api/countdown` agrees the state changed.
- The image's default command must start the server (`serve`): OpenChoreo
  cannot set command/args.
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
