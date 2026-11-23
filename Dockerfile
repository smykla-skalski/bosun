# syntax=docker/dockerfile:1.29@sha256:572cebbc2b15e6f84c077742952674889cb63cd5e9b80abdffcbe587b6f19183

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.23@sha256:f70850ef6ce341e8b6427f677049145d63c0b7ce002d4d7e3d464828b1b96dcd AS build

ARG TARGETOS=linux
ARG TARGETARCH

ENV GOTOOLCHAIN=local

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download

COPY assets/catalog.go assets/replies.pl.json ./assets/
COPY cmd ./cmd
COPY internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" \
    go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o /out/bosun ./cmd/bosun

FROM scratch

LABEL org.opencontainers.image.title="Bosun" \
      org.opencontainers.image.description="Private Discord bot for one configured server" \
      org.opencontainers.image.source="https://github.com/smykla-skalski/bosun"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65532:65532 /out/bosun /bosun

USER 65532:65532
EXPOSE 8080

ENTRYPOINT ["/bosun"]
