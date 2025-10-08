# Makefile for WireGuard userspace playground

.PHONY: all build clean server client test combined help

# Default target
all: build

# Build both server and client
build:
	@echo "Building WireGuard playground..."
	go build -o wg-server server.go
	go build -o wg-client client.go
	@echo "✓ Built wg-server and wg-client"

# Build combined binary
combined:
	@echo "Building combined WireGuard test..."
	go build -o wg-combined combined.go
	@echo "✓ Built wg-combined"
	@echo "Running combined test..."
	./wg-combined

# Run the server
server: build
	@echo "Starting WireGuard server (192.168.4.2:80)..."
	./wg-server

# Run the client (assumes server is already running)
client: build
	@echo "Starting WireGuard client (192.168.4.1)..."
	./wg-client

# Run full test: start server in background, run client, stop server
test: build
	@echo "Testing WireGuard connection..."
	@./wg-server & \
	SERVER_PID=$$! ; \
	sleep 2 ; \
	./wg-client ; \
	kill $$SERVER_PID 2>/dev/null || true

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f wg-server wg-client wg-combined wgplay
	@echo "✓ Cleaned"

# Show help
help:
	@echo "WireGuard Playground Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build    - Build server and client binaries"
	@echo "  server   - Run the WireGuard server (192.168.4.2:80)"
	@echo "  client   - Run the WireGuard client (192.168.4.1)"
	@echo "  test     - Run full test (server + client)"
	@echo "  combined - Build and run combined single-binary test"
	@echo "  clean    - Remove built binaries"
	@echo "  help     - Show this help"
	@echo ""
	@echo "Usage:"
	@echo "  Terminal 1: make server"
	@echo "  Terminal 2: make client"
	@echo ""
	@echo "  Or run: make test"
	@echo "  Or run: make combined"
