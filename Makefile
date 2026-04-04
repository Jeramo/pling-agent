VERSION ?= dev
LDFLAGS = -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build clean all tray

build:
	go build $(LDFLAGS) -o pling-agent ./cmd/pling-agent

tray:
	CGO_ENABLED=1 go build -ldflags "-s -w" -o dist/pling-tray-darwin-arm64 ./cmd/pling-tray

all:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-linux-amd64 ./cmd/pling-agent
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/pling-agent-linux-arm64 ./cmd/pling-agent
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-darwin-amd64 ./cmd/pling-agent
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/pling-agent-darwin-arm64 ./cmd/pling-agent
	GOOS=freebsd GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-freebsd-amd64 ./cmd/pling-agent
	$(MAKE) tray

clean:
	rm -rf pling-agent pling-tray dist/
