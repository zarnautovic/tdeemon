# syntax=docker/dockerfile:1

# tdeemon: one static binary with the web app inside, on distroless.
#
#   docker build -t tdeemon .
#   docker run --read-only -v tdeemon-data:/data -p 8080:8080 \
#     -e TDEEMON_BASE_URL=http://localhost:8080 \
#     -e TDEEMON_SECRET_KEY="$(cat secret_key)" tdeemon
#
# README.md ("Docker") has the details.
#
# The web and build stages run on the build machine's platform and Go
# cross-compiles for the target, so a multi-arch build needs no emulation.

FROM --platform=$BUILDPLATFORM docker.io/library/node:24.22.0-trixie-slim@sha256:7f486cabe892bff8fef04c0f7da8dba84a560be3e4e2740b7a86afc57ebec315 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY web/ ./
# Writes ../internal/webui/dist, which the Go build embeds.
RUN npm run build

FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.27.2-trixie@sha256:47667934874913a449b96faa7ef4777fa285d9ec29126ee80ed61c45b7cccf0e AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=web /src/internal/webui/dist internal/webui/dist
# The tests run against the web app this image ships, so the embed checks
# see the real build.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go vet -tags webui ./... && go test -tags webui ./...
ARG TARGETOS TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -tags webui -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/tdeemon ./cmd/tdeemon \
 && mkdir /out/data

FROM gcr.io/distroless/static-debian13:nonroot@sha256:516757106de35205efb70b91ba03f6f1298424a0678c603255cf2852a7000087
ARG VERSION=dev
LABEL org.opencontainers.image.title="tdeemon" \
      org.opencontainers.image.description="Self-hosted, AI-first nutrition tracking with an adaptive TDEE" \
      org.opencontainers.image.source="https://github.com/zarnautovic/tdeemon" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}"
COPY --from=build /out/tdeemon /tdeemon
# The data directory belongs to the nonroot user; an empty named volume
# mounted over it takes that ownership. There is no VOLUME instruction on
# purpose: mount one explicitly, or with --read-only the start fails.
COPY --from=build --chown=65532:65532 /out/data /data
ENV TDEEMON_DATA_DIR=/data
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/tdeemon"]
