.PHONY: all build clean serve help

# Default target
all: build

# Build the WASM module
# GOOS=js GOARCH=wasm tells Go to compile for WebAssembly
# -o wasm/main.wasm specifies output file
build:
	@echo "Building WASM module..."
	GOOS=js GOARCH=wasm go build -o wasm/main.wasm ./wasm/main.go
	@echo "✓ Built wasm/main.wasm"
	@echo ""
	@echo "Copying wasm_exec.js from Go installation..."
	@# wasm_exec.js is the Go->WASM runtime support file
	@# It's part of Go's standard distribution
	@# We need to copy it to our directory so the browser can load it
	@# Location changed in Go 1.21+: now in lib/wasm/ instead of misc/wasm/
	@if [ -f "$$(go env GOROOT)/lib/wasm/wasm_exec.js" ]; then \
		cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" ./wasm_exec.js; \
	else \
		cp "$$(go env GOROOT)/misc/wasm/wasm_exec.js" ./wasm_exec.js; \
	fi
	@echo "✓ Copied wasm_exec.js"

# Start a local web server to test the WASM module
# WASM files must be served with the correct MIME type
# Python's http.server does this correctly
serve:
	@echo "Starting web server on http://localhost:8080"
	@echo "Press Ctrl+C to stop"
	@cd . && python3 -m http.server 8080

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f wasm/main.wasm wasm_exec.js
	@echo "✓ Cleaned"

# Show help
help:
	@echo "Spanza Browser WASM Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build  - Build the WASM module"
	@echo "  serve  - Start local web server on port 8080"
	@echo "  clean  - Remove build artifacts"
	@echo "  help   - Show this help"
	@echo ""
	@echo "Usage:"
	@echo "  make build   - Build WASM"
	@echo "  make serve   - Test in browser"
