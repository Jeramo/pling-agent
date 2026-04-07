VERSION ?= dev
LDFLAGS = -ldflags "-s -w -X main.version=$(VERSION)"
SIGN_ID ?= Developer ID Application: Jean-Robert Nino (7W75D5ZQ4C)
TEAM_ID = 7W75D5ZQ4C

.PHONY: build clean all sign notarize release

build:
	go build $(LDFLAGS) -o pling-agent ./cmd/pling-agent

all:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-linux-amd64 ./cmd/pling-agent
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/pling-agent-linux-arm64 ./cmd/pling-agent
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-darwin-amd64 ./cmd/pling-agent
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/pling-agent-darwin-arm64 ./cmd/pling-agent
	GOOS=freebsd GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-freebsd-amd64 ./cmd/pling-agent
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/pling-agent-windows-amd64.exe ./cmd/pling-agent

# Sign macOS binaries with Developer ID
sign:
	@echo "Signing macOS binaries..."
	codesign --sign "$(SIGN_ID)" --timestamp --options runtime --force dist/pling-agent-darwin-arm64
	codesign --sign "$(SIGN_ID)" --timestamp --options runtime --force dist/pling-agent-darwin-amd64
	@echo "All macOS binaries signed"

# Notarize macOS binaries with Apple
notarize:
	@echo "Notarizing macOS binaries..."
	@for bin in dist/pling-agent-darwin-arm64 dist/pling-agent-darwin-amd64; do \
		echo "Submitting $$bin..."; \
		zip -j "$$bin.zip" "$$bin"; \
		xcrun notarytool submit "$$bin.zip" \
			--key ~/.appstoreconnect/private_keys/AuthKey_5GR32QXQQZ.p8 \
			--key-id 5GR32QXQQZ \
			--issuer 9eeba692-a6da-41bd-95d3-7e62e1cf674c \
			--wait; \
		rm "$$bin.zip"; \
	done
	@echo "All macOS binaries notarized"

# Full release: build all → sign → notarize → upload to GitHub
release: all sign notarize
	@if [ "$(VERSION)" = "dev" ]; then echo "Set VERSION, e.g. make release VERSION=v0.9.7"; exit 1; fi
	@echo "Creating GitHub release $(VERSION)..."
	gh release create $(VERSION) dist/* --title "$(VERSION)" --generate-notes
	@echo "Release $(VERSION) published"

clean:
	rm -rf pling-agent dist/
