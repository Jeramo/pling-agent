VERSION ?= dev
LDFLAGS = -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build clean all

build:
	go build $(LDFLAGS) -o pling-agent ./cmd/pling-agent

all:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-linux-amd64 ./cmd/pling-agent
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/pling-agent-linux-arm64 ./cmd/pling-agent
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-darwin-amd64 ./cmd/pling-agent
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/pling-agent-darwin-arm64 ./cmd/pling-agent
	GOOS=freebsd GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-freebsd-amd64 ./cmd/pling-agent

clean:
	rm -rf pling-agent dist/
