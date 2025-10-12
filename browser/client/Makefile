.PHONY: build run clean

# Build the native client
build:
	go build -o client main.go

# Run the client
run:
	go run main.go

# Clean build artifacts
clean:
	rm -f client

# Help
help:
	@echo "Available targets:"
	@echo "  make build  - Build the native client binary"
	@echo "  make run    - Run the client directly with go run"
	@echo "  make clean  - Remove build artifacts"
