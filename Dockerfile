FROM golang:1.25-alpine AS builder

WORKDIR /build

# Cache deps before copying source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG COMMIT_DATE=unknown
ARG TREE_STATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
      -X main.Version=${VERSION} \
      -X main.Commit=${COMMIT} \
      -X main.CommitDate=${COMMIT_DATE} \
      -X main.TreeState=${TREE_STATE}" \
    -o helmtrace \
    .

# ── runtime ──────────────────────────────────────────────────────────────────
FROM scratch

COPY --from=builder /build/helmtrace /helmtrace

LABEL org.opencontainers.image.source="https://github.com/jornh/helmtrace"
LABEL org.opencontainers.image.description="Trace provenance of values across layered Helm values files and Kustomize overlays"
LABEL org.opencontainers.image.licenses="MIT"

ENTRYPOINT ["/helmtrace"]
