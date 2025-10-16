# Spanza (Sancho Panza)

Exploratory work on using [DERP](https://tailscale.com/blog/how-tailscale-works#encrypted-tcp-relays-derp) (Designated Encrypted Relay for Packets) servers
as the transport mechanism instead of UDP. You may find [miniwg](https://github.com/drio/miniwg) also
useful.

[WireGuard](https://www.wireguard.com/) is fantastic but it requires a direct
UDP connection between peers. You may run wg in environments where UDP is
completely blocked. Yes, you could add NAT/Firewall traversal techniques but
what will always work is using a DERP server to route the traffic (at the cost of network performance).

There is currently a lot of exploratory work in userspace but you have a working version of a sidecar
(stanza) which you can use to send your wg traffic over DERP. You just need to run the stanza process
and point your wg configuration to use stanza instead of the peer endpoint.

Still under heavy development but I wanted to put it out there to get feedback. It is already functional
and useful.

### WASM Implementation

This project also includes a **WebAssembly implementation** that runs WireGuard
entirely in the browser using DERP as transport. It is broken and I am still
trying to figure out what is going on. 

### Using Tailscale's DERP Servers

In my tests I am using [Tailscale's DERP
servers](https://login.tailscale.com/derpmap/default). Tailscale has rate
limits and other techniques to avoid abuse. Be respectful and consider [running
your own DERP server](https://tailscale.com/kb/1118/custom-derp-servers) if you
are going to send a lot of data.

Tailscale is great, the folks are good people and you should consider using them.

### References

- [WireGuard whitepaper](https://www.wireguard.com/papers/wireguard.pdf)
- [Tailscale DERP documentation](https://tailscale.com/blog/how-tailscale-works)
- [WireGuard Go implementation](https://git.zx2c4.com/wireguard-go/)
- [Tailscale source code](https://github.com/tailscale/tailscale)
