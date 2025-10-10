.PHONY: all build test clean help

# Default target
all: build

# Build the userspace test
build:
	@echo "Building userspace WireGuard + Spanza test..."
	go build -o ustest ustest.go
	@echo "✓ Built ustest"

# Run the test
test: build
	@echo "Running userspace test..."
	./ustest

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f ustest peer1 peer2
	@echo "✓ Cleaned"

# Show help
help:
	@echo "Userspace WireGuard + Spanza Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build  - Build the ustest binary"
	@echo "  test   - Build and run the test"
	@echo "  clean  - Remove built binaries"
	@echo "  help   - Show this help"
	@echo ""
	@echo "Usage:"
	@echo "  make test    - Run the combined test"
	@echo "  ./ustest     - Run directly"
