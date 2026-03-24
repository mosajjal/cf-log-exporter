BIN := cf-log-exporter

.PHONY: build lint test install

build:
	go build -o $(BIN) .

lint:
	go vet ./...
	golangci-lint run

test:
	go test -race ./...

install: build
	install -Dm755 $(BIN) /usr/local/bin/$(BIN)
	install -Dm644 $(BIN).service /etc/systemd/system/$(BIN).service
	@echo "Run: systemctl daemon-reload && systemctl enable --now $(BIN)"
