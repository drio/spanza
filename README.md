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

Still under heavy development but I wanted to put it out there to get feedback. It also already functional
and useful.
