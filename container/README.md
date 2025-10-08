# Container Testing

Simple container environment for testing WireGuard relay.

## Usage

```bash
cd container

# Build image
make build

# Terminal 1: Start peer1
make peer1
# Inside: RELAY_ENDPOINT=drio.sh:51820 ./playground/wg/wg-server

# Terminal 2: Start peer2
make peer2
# Inside: RELAY_ENDPOINT=drio.sh:51820 ./playground/wg/wg-client
```

Test with ping, curl, tcpdump, etc.

## Available tools

- ping, curl, wget, tcpdump, netcat, dig
- vim, nano
