FROM golang:1.21 as build
WORKDIR /src
COPY . .
RUN go mod download
RUN GOARCH=amd64 go build -o /usr/bin/ingest

FROM stellar/stellar-core:latest
COPY --from=build /usr/bin/ingest /usr/bin/ingest
COPY --from=build /src/stellar-core.toml /etc/stellar/stellar-core.toml
ENTRYPOINT ["/usr/bin/ingest"]