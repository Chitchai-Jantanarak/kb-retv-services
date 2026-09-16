# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.2
ARG TOKENIZERS_VERSION=v1.27.0

FROM golang:${GO_VERSION}-bookworm AS base

WORKDIR /app

RUN go install github.com/air-verse/air@v1.67.4

COPY go.mod go.sum ./
RUN go mod download

FROM base AS dev

ENV CGO_ENABLED=0

CMD ["air", "-c", ".air.toml"]

FROM base AS tokenizers

ARG TOKENIZERS_VERSION
ADD https://github.com/daulet/tokenizers/releases/download/${TOKENIZERS_VERSION}/libtokenizers.linux-amd64.tar.gz /tmp/libtokenizers.tar.gz
RUN tar -xzf /tmp/libtokenizers.tar.gz -C /usr/lib libtokenizers.a \
    && rm /tmp/libtokenizers.tar.gz

ENV CGO_ENABLED=1

CMD ["air", "-c", ".air.tokenizers.toml"]

# Runtime image. CGO off, so the guard embedder falls back to the stub factory
# (staticlocal_factory_stub.go) - build the `tokenizers` variant separately if a
# deployment needs the real tokenizer.
#
# config.yaml and config/tools are read from disk at runtime relative to the
# working directory: cmd/api/chat.go:179 does os.DirFS("config/tools").
FROM base AS build-prod

ENV CGO_ENABLED=0

COPY . .

RUN go build -trimpath -ldflags="-s -w" -o /out/api       ./cmd/api \
    && go build -trimpath -ldflags="-s -w" -o /out/worker    ./cmd/worker \
    && go build -trimpath -ldflags="-s -w" -o /out/scheduler ./cmd/scheduler

FROM gcr.io/distroless/static-debian12:nonroot AS prod

WORKDIR /app

COPY --from=build-prod /out/api /out/worker /out/scheduler /usr/local/bin/
COPY config.yaml ./config.yaml
COPY config/tools ./config/tools

USER nonroot

EXPOSE 8080

CMD ["api"]
