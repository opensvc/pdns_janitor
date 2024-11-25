FROM golang:1.21 as build

LABEL maintainer="support@opensvc.com"
LABEL org.opencontainers.image.source="https://github.com/opensvc/pdns_janitor"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.description="A backend for a pdns server serving records for the services deployed in a OpenSVC cluster."

WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o pdns_janitor

FROM scratch as final
COPY --from=build /build/pdns_janitor /
CMD ["/pdns_janitor"]

