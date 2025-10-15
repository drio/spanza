.PHONY: all build run clean help

# Default target
all: build

# Build the server peer
build:
	@echo "Building WireGuard server peer..."
	go build -o server main.go
	@echo "✓ Built server binary"

# Run the server
run: build
	@echo "Starting server peer..."
	@echo ""
	./server

# Run the server (no wireguard)
run/derp-only: build
	@echo "Starting server peer (derp only; no wireguard)..."
	@echo ""
	./server --test-derp


# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f server
	@echo "✓ Cleaned"

# Show help
help:
	@echo "WireGuard Server Peer Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build  - Build the server binary"
	@echo "  run    - Build and run the server"
	@echo "  clean  - Remove build artifacts"
	@echo "  help   - Show this help"
	@echo ""
	@echo "Usage:"
	@echo "  make run    - Start the server"
	@echo "  ./server    - Run directly"
