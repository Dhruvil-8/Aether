FROM golang:1.23 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/aetherd ./cmd/aetherd

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /out/aetherd /usr/local/bin/aetherd

ENV AETHER_DATA_DIR=/var/lib/aether
EXPOSE 9000

ENTRYPOINT ["/usr/local/bin/aetherd"]
CMD ["serve"]
