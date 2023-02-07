FROM golang:1.19 as build
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o pdns_janitor

FROM scratch as final
COPY --from=build /build/pdns_janitor /
CMD ["/pdns_janitor"]

