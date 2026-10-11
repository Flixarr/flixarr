# Flixarr: one small image with the Go server and the built web app inside.

FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.23-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /web/build ./internal/webui/dist
ARG VERSION=0.1.0
ARG BUILD=0
ARG COMMIT=dev
ARG CHANNEL=nightly
ARG DEV=
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/Flixarr/flixarr/internal/version.Version=${VERSION} -X github.com/Flixarr/flixarr/internal/version.Build=${BUILD} -X github.com/Flixarr/flixarr/internal/version.Commit=${COMMIT} -X github.com/Flixarr/flixarr/internal/version.Channel=${CHANNEL} -X github.com/Flixarr/flixarr/internal/version.Dev=${DEV}" \
    -o /flixarr ./cmd/flixarr

FROM alpine:3
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 1000 flixarr \
 && mkdir -p /data && chown flixarr:flixarr /data
COPY --from=server /flixarr /usr/local/bin/flixarr
USER flixarr
ENV FLIXARR_DATA=/data FLIXARR_PORT=7575
EXPOSE 7575
VOLUME /data
HEALTHCHECK --interval=30s --timeout=5s CMD wget -qO- http://127.0.0.1:7575/api/status >/dev/null || exit 1
ENTRYPOINT ["flixarr"]
