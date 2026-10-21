# 🎂 Birthday

A little countdown to someone's birthday, with an I.O.U. card that says
*"I owe you a birthday drink"*. On the day itself the page switches to a
*Happy birthday* greeting with confetti. Click the champagne glass for a toast.

It's one static Go binary (standard library only, with the template, assets
and time zone database embedded), configured entirely through environment
variables.

## Configuration

| Variable              | Required | Default                      | Description                                                            |
| --------------------- | -------- | ---------------------------- | ---------------------------------------------------------------------- |
| `BIRTHDAY_NAME`       | yes      |                              | Who the countdown is for (max 64 characters).                          |
| `BIRTHDAY_DATE`       | yes      |                              | `MM-DD`, or `YYYY-MM-DD` to also show the age they are turning.        |
| `BIRTHDAY_MESSAGE`    | no       | `I owe you a birthday drink` | The line on the I.O.U. card (max 280 characters).                      |
| `BIRTHDAY_FROM`       | no       |                              | Signature under the line.                               |
| `BIRTHDAY_TIMEZONE`   | no       | `UTC`                        | IANA zone used for midnight, e.g. `Europe/Paris`.                      |
| `BIRTHDAY_HALLOWEEN`  | no       | `true`                       | Subtle seasonal touches: a crescent moon, two bats and a corner cobweb. |
| `BIRTHDAY_HOST`       | no       | all interfaces               | Listen address.                                                        |
| `BIRTHDAY_PORT`       | no       | `8080`                       | Listen port.                                                           |

A 29 February birthday is celebrated on 28 February in non-leap years. An
invalid configuration stops the server at startup and lists every problem.

## Endpoints

- `GET /`: the countdown page.
- `GET /api/countdown`: the same state as JSON (`start`, `end`, `today`,
  `turning`, `now`, …).
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
   for that tag and publishes the workload.
3. The name, date, line and hostname are set per environment in hestia's
   ReleaseBindings.
