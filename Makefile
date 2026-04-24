# HAOS Network Readiness Checker - Build Makefile

BINARY_NAME=haos-check
VERSION=1.2.0
BUILD_DIR=build

# Build flags for smaller binaries
LDFLAGS=-s -w -X main.version=$(VERSION)

.PHONY: all clean build build-all test run

# Default: build for current platform
all: build

# Build for current OS/arch
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

# Run the checker
run: build
	./$(BINARY_NAME)

# Run with verbose output
run-verbose: build
	./$(BINARY_NAME) -v

# Build for all platforms (cross-compilation)
build-all: clean
	@mkdir -p $(BUILD_DIR)
	@echo "Building for all platforms..."

	@# Linux AMD64 (most servers, Intel NUCs)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	@echo "  ✓ Linux AMD64"

	@# Linux ARM64 (Raspberry Pi 4/5, Orange Pi, etc.)
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 .
	@echo "  ✓ Linux ARM64"

	@# Linux ARMv7 (Raspberry Pi 3, older ARM boards)
	GOOS=linux GOARCH=arm GOARM=7 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-armv7 .
	@echo "  ✓ Linux ARMv7"

	@# Linux ARMv6 (Raspberry Pi Zero, Pi 1)
	GOOS=linux GOARCH=arm GOARM=6 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-armv6 .
	@echo "  ✓ Linux ARMv6"

	@# macOS AMD64 (Intel Macs)
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	@echo "  ✓ macOS AMD64"

	@# macOS ARM64 (Apple Silicon)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	@echo "  ✓ macOS ARM64"

	@# Windows AMD64
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .
	@echo "  ✓ Windows AMD64"

	@# Windows ARM64
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe .
	@echo "  ✓ Windows ARM64"

	@echo ""
	@echo "All binaries built in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/

# Create release archives
release: build-all
	@echo "Creating release archives..."
	@cd $(BUILD_DIR) && \
	for f in $(BINARY_NAME)-linux-* $(BINARY_NAME)-darwin-*; do \
		tar -czf $$f.tar.gz $$f && rm $$f; \
	done
	@cd $(BUILD_DIR) && \
	for f in $(BINARY_NAME)-windows-*.exe; do \
		zip -q $${f%.exe}.zip $$f && rm $$f; \
	done
	@echo "Release archives created:"
	@ls -lh $(BUILD_DIR)/

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)

# Run tests
test:
	go test -v ./...

# Check code
lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed" && exit 1)
	golangci-lint run

# Show help
help:
	@echo "HAOS Network Readiness Checker - Build Targets"
	@echo ""
	@echo "  make build      - Build for current platform"
	@echo "  make build-all  - Build for all platforms (Linux, macOS, Windows)"
	@echo "  make release    - Build all + create release archives"
	@echo "  make run        - Build and run"
	@echo "  make run-verbose - Build and run with verbose output"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make help       - Show this help"
