FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o cf-log-exporter .

FROM alpine:3
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/cf-log-exporter /usr/local/bin/cf-log-exporter
ENTRYPOINT ["cf-log-exporter"]
