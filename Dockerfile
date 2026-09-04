# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.2
ARG AIR_VERSION=v1.67.4
ARG TOKENIZERS_VERSION=v1.27.0

FROM golang:${GO_VERSION}-bookworm AS base

WORKDIR /app

ARG AIR_VERSION
RUN go install github.com/air-verse/air@${AIR_VERSION}

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
