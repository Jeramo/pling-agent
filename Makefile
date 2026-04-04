VERSION ?= dev
LDFLAGS = -ldflags "-s -w -X main.version=$(VERSION)"
TRAY_LDFLAGS = -ldflags "-s -w -H windowsgui"

.PHONY: build clean all tray

build:
	go build $(LDFLAGS) -o pling-agent ./cmd/pling-agent

tray:
	CGO_ENABLED=1 go build -ldflags "-s -w" -o dist/pling-tray-darwin-arm64 ./cmd/pling-tray
	CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build $(TRAY_LDFLAGS) -o dist/pling-tray-windows-amd64.exe ./cmd/pling-tray

all:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-linux-amd64 ./cmd/pling-agent
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/pling-agent-linux-arm64 ./cmd/pling-agent
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-darwin-amd64 ./cmd/pling-agent
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/pling-agent-darwin-arm64 ./cmd/pling-agent
	GOOS=freebsd GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-freebsd-amd64 ./cmd/pling-agent
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-windows-amd64.exe ./cmd/pling-agent
	$(MAKE) tray

clean:
	rm -rf pling-agent pling-tray dist/
