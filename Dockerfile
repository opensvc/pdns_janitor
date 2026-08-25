
# syntax=docker/dockerfile:1

FROM golang:alpine3.24@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o pdns_janitor

FROM scratch AS final
COPY --from=builder /src/pdns_janitor /
CMD ["/pdns_janitor"]

