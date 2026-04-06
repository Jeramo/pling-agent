VERSION ?= dev
LDFLAGS = -ldflags "-s -w -X main.version=$(VERSION)"
TRAY_LDFLAGS = -ldflags "-s -w -H windowsgui"
SIGN_ID ?= Developer ID Application: Jean-Robert Nino (7W75D5ZQ4C)
APPLE_ID ?= jeanrobert.nino.layton@gmail.com
TEAM_ID = 7W75D5ZQ4C

.PHONY: build clean all tray sign notarize release

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

# Sign macOS binaries with Developer ID
sign:
	@echo "Signing macOS binaries..."
	codesign --sign "$(SIGN_ID)" --timestamp --options runtime --force dist/pling-agent-darwin-arm64
	codesign --sign "$(SIGN_ID)" --timestamp --options runtime --force dist/pling-agent-darwin-amd64
	codesign --sign "$(SIGN_ID)" --timestamp --options runtime --force dist/pling-tray-darwin-arm64
	@echo "All macOS binaries signed"

# Notarize macOS binaries with Apple
notarize:
	@echo "Notarizing macOS binaries..."
	@for bin in dist/pling-agent-darwin-arm64 dist/pling-agent-darwin-amd64 dist/pling-tray-darwin-arm64; do \
		echo "Submitting $$bin..."; \
		zip -j "$$bin.zip" "$$bin"; \
		xcrun notarytool submit "$$bin.zip" \
			--apple-id "$(APPLE_ID)" --team-id "$(TEAM_ID)" \
			--keychain-profile "pling-notary" --wait; \
		rm "$$bin.zip"; \
	done
	@echo "All macOS binaries notarized"

# Generate SHA-256 checksums
checksums:
	@cd dist && for f in pling-agent-* pling-tray-*; do \
		[ -f "$$f" ] && shasum -a 256 "$$f" > "$$f.sha256"; \
	done
	@echo "Checksums generated"

# Full release: build all → sign → notarize → checksums → upload to GitHub
release: all sign notarize checksums
	@if [ "$(VERSION)" = "dev" ]; then echo "Set VERSION, e.g. make release VERSION=v0.9.7"; exit 1; fi
	@echo "Creating GitHub release $(VERSION)..."
	gh release create $(VERSION) dist/* --title "$(VERSION)" --generate-notes
	@echo "Release $(VERSION) published"

clean:
	rm -rf pling-agent pling-tray dist/
