# syntax=docker/dockerfile:1
# Same layout as nix/oci.nix: /birthday (template, assets and zone database
# are embedded), Claude Code for the chat friend and a shell to sign it in
# (`claude auth login`), user 1001:0 with a writable home, default command
# `serve`.
FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X github.com/Banh-Canh/birthday/cmd.Version=${VERSION} -X github.com/Banh-Canh/birthday/cmd.Commit=${COMMIT}" \
      -o /out/birthday .

# Claude Code's native build (glibc only), pinned.
FROM debian:bookworm-slim AS claude
ARG CLAUDE_VERSION=2.1.308
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && curl -fsSL https://claude.ai/install.sh | bash -s "${CLAUDE_VERSION}" \
    && cp -L /root/.local/bin/claude /claude

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 1001 --gid 0 --home-dir /home/birthday --create-home --shell /bin/bash birthday \
    && chmod 0770 /home/birthday
COPY --from=build /out/birthday /birthday
COPY --from=claude /claude /usr/local/bin/claude
# The chat friend's own Claude login lives here, apart from any other
# account settings: sign in with `claude auth login` in the container.
ENV HOME=/home/birthday \
    CLAUDE_CONFIG_DIR=/home/birthday/.claude \
    DISABLE_AUTOUPDATER=1
USER 1001:0
WORKDIR /home/birthday
EXPOSE 8080
ENTRYPOINT ["/birthday"]
CMD ["serve"]
