# Tailscale Public DERP Servers

Tailscale operates a global network of public DERP servers that anyone can use
for testing. These servers are **completely open** - no authentication required.

## Why Use Tailscale's DERP Servers?

- **No setup required** - Test immediately without running your own server
- **Production-grade** - Running on port 443 with TLS
- **Global coverage** - Test from different geographic regions
- **Free to use** - For testing and development

**Note**: Tailscale's public DERP servers are provided as-is for the community.
Don't abuse them - for production use, run your own DERP server.

## Available Servers

Tailscale operates DERP servers in multiple regions. Here are some you can use:

### North America

| Region | Hostname | URL |
|--------|----------|-----|
| New York | derp.tailscale.com | https://derp.tailscale.com/derp |
| San Francisco | derp1.tailscale.com | https://derp1.tailscale.com/derp |

### Europe

| Region | Hostname | URL |
|--------|----------|-----|
| Frankfurt | derp3.tailscale.com | https://derp3.tailscale.com/derp |
| London | derp4.tailscale.com | https://derp4.tailscale.com/derp |

### Asia Pacific

| Region | Hostname | URL |
|--------|----------|-----|
| Tokyo | derp5.tailscale.com | https://derp5.tailscale.com/derp |
| Singapore | derp9.tailscale.com | https://derp9.tailscale.com/derp |

### Other Regions

| Region | Hostname | URL |
|--------|----------|-----|
| Sydney | derp6.tailscale.com | https://derp6.tailscale.com/derp |
| Bangalore | derp10.tailscale.com | https://derp10.tailscale.com/derp |

**Note**: Tailscale's server list changes over time. For the most current list,
check: https://login.tailscale.com/derpmap/default

## Using with This Playground

### With Makefile Targets

```bash
# Terminal 1: Start Client B (receiver)
make run-clientB-ts

# Terminal 2: Start Client A (sender)
make run-clientA-ts
```

### Manual Testing

```bash
# Test with New York server
./bin/clientB --server https://derp.tailscale.com/derp --key <key> --echo

# Test with San Francisco server
./bin/clientB --server https://derp1.tailscale.com/derp --key <key> --echo

# Test with Tokyo server
./bin/clientB --server https://derp5.tailscale.com/derp --key <key> --echo
```

## Testing Different Regions

You can test relay performance between regions:

```bash
# Terminal 1: Client B in "Tokyo"
./bin/clientB --server https://derp5.tailscale.com/derp --key <key> --echo

# Terminal 2: Client A in "New York"
./bin/clientA --server https://derp5.tailscale.com/derp --key <key> --peer <pubkey>
```

This simulates clients in different geographic regions using the same relay.

## Finding the Full DERP Map

Tailscale publishes their DERP map at:

```bash
curl https://login.tailscale.com/derpmap/default | jq .
```

This JSON includes:
- All active DERP servers
- Geographic locations
- IPv4 and IPv6 addresses
- Port configurations
- STUN endpoints

Example output:
```json
{
  "Regions": {
    "1": {
      "RegionID": 1,
      "RegionCode": "nyc",
      "RegionName": "New York City",
      "Nodes": [
        {
          "Name": "1a",
          "RegionID": 1,
          "HostName": "derp.tailscale.com",
          "IPv4": "64.227.41.159",
          "IPv6": "2604:a880:400:d0::1b2f:6001",
          "STUNPort": 3478,
          "DERPPort": 443
        }
      ]
    }
  }
}
```

## What You Can Learn

Using Tailscale's public DERP servers lets you:

1. **Test without infrastructure** - No need to set up your own server
2. **Understand production deployment** - See how DERP runs on port 443 with TLS
3. **Test geographic distribution** - Connect clients through different regions
4. **Verify client implementation** - Ensure your code works with real DERP servers
5. **Measure latency** - Compare relay latency across regions

## Privacy and Security Notes

### What Tailscale Can See

When you use their DERP servers:
- **They can see**: Your client's public key, connection times, bandwidth usage
- **They CANNOT see**: The content of your messages (end-to-end encrypted)
- **They CANNOT see**: Who you're talking to (peer public keys are encrypted in DERP frames)

### Rate Limiting

Tailscale's servers likely have rate limiting to prevent abuse. If you're doing
heavy testing:
- Use your own DERP server
- Be respectful of the shared resource
- Don't hammer the servers with excessive traffic

### No SLA

These are public servers provided as-is:
- No uptime guarantee
- May change or disappear
- Not suitable for production applications

## Comparison: Public vs Your Own

| Aspect | Tailscale Public | Your DERP Server |
|--------|------------------|------------------|
| **Setup** | None | Build & deploy |
| **Cost** | Free | Server costs |
| **Control** | None | Full control |
| **Privacy** | Shared | Private |
| **SLA** | None | Your responsibility |
| **Access Control** | Open to all | You decide |
| **Customization** | Not possible | Full flexibility |

## When to Use Each

### Use Tailscale's Public DERP for:
- ✓ Learning and experimentation
- ✓ Quick prototyping
- ✓ Testing client implementations
- ✓ Demos and tutorials
- ✓ Open source projects (testing CI)

### Use Your Own DERP for:
- ✓ Production applications
- ✓ Privacy-sensitive applications
- ✓ High-bandwidth applications
- ✓ Custom authentication needs
- ✓ Geographic requirements (specific regions)

## Example: Testing Multi-Region

```bash
# In one region, use New York
./bin/clientB --server https://derp.tailscale.com/derp --key $b --echo &

# In another region, use Tokyo
./bin/clientB --server https://derp5.tailscale.com/derp --key $c --echo &

# Client A can relay to both through the same server
./bin/clientA --server https://derp.tailscale.com/derp --key $a --peer <pubkey-of-b>
./bin/clientA --server https://derp.tailscale.com/derp --key $a --peer <pubkey-of-c>
```

**Note**: Both clients must use the **same** DERP server to communicate.
DERP servers don't relay to each other unless configured in a mesh.

## Acknowledgment

These public DERP servers are provided by Tailscale as a service to the
community. Thank you Tailscale! 🙏

If you use them for testing, consider:
- Using your own server for production
- Contributing back to the open source community
- Respecting the shared resource

## References

- Tailscale DERP documentation: https://tailscale.com/kb/1232/derp-servers
- DERP map endpoint: https://login.tailscale.com/derpmap/default
- DERP protocol docs: https://pkg.go.dev/tailscale.com/derp
- How to run your own: https://github.com/tailscale/tailscale/tree/main/cmd/derper
