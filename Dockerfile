# syntax=docker/dockerfile:1.20@sha256:26147acbda4f14c5add9946e2fd2ed543fc402884fd75146bd342a7f6271dc1d

FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine3.23@sha256:9e45f0eb4a63ed37ad6e604950407558b7c738e34e015ae77a5d4ad369cde079 AS build

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
