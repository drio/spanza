# Simple container testing

IMAGE := spanza:latest

build:
	@echo "Building spanza binary on host..."
	cd .. && go build -o spanza .
	@echo "Building container image..."
	podman build -t $(IMAGE) -f Containerfile ..

# NET_ADMIN capability needed to create/configure WireGuard interfaces
# NET_RAW: needed for tools that send or inspect raw packets (ping, wg, etc.)
peer1: build
	podman run -it  \
		--rm --name peer1 --hostname peer1  \
 		--cap-add=NET_ADMIN --cap-add=NET_RAW \
		--env-file .env $(IMAGE) /bin/bash 

peer2: build
	podman run -it \
		--rm --name peer2 --hostname peer2 \
		--cap-add=NET_ADMIN --cap-add=NET_RAW \
		--env-file .env $(IMAGE) /bin/bash

clean:
	podman rmi $(IMAGE)
