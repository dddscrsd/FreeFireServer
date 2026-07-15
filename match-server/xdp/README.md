# XDP per-source rate limiter

Drops a multi-source UDP flood **in the NIC driver**, before the kernel network stack — so junk packets
cost almost no CPU and generate no ICMP replies (which is what was filling your egress). Pairs with the
`SO_REUSEPORT` multi-socket receive loop in the Go server: XDP culls the flood, `SO_REUSEPORT` spreads
whatever's left across cores.

## What it does
For each inbound IPv4/UDP datagram to `GAME_PORT` (default **10100**), it counts packets per **source IP**
in a 1s window. Over `RATE_MAX` (default **100/s**; a real client is ~35/s) → `XDP_DROP`. Everything else
— other ports (SSH), TCP, ICMP, IPv6, IP-options packets — passes untouched. State lives in an LRU BPF
map capped at ~1M sources, so a spoofed flood can't exhaust memory.

## Build + load
```bash
# deps (once): Debian/Ubuntu
sudo apt install clang llvm libbpf-dev linux-headers-$(uname -r) iproute2

make                                   # -> xdp_ratelimit.o
sudo make load   IFACE=eth0            # native (driver) mode — fastest
#   if that errors with "native XDP not supported by driver":
sudo make load   IFACE=eth0 MODE=skb   # generic/skb mode — works on ANY driver, still pre-stack

sudo make status IFACE=eth0            # confirm it's attached
sudo make dump                         # peek at per-source counters
sudo make unload IFACE=eth0            # detach
```
Find your interface with `ip -br link` / `ethtool -i <iface>`. On cloud VMs the driver is usually
`ena` (AWS), `gve` (GCP) or `virtio_net` — all do native XDP on recent kernels; `MODE=skb` is the safe
universal fallback.

## Tuning
`GAME_PORT` and `RATE_MAX` are **build params from the env** — the Makefile passes them to clang as `-D`,
so you don't edit the `.c`:
```bash
GAME_PORT=12345 RATE_MAX=150 sudo make load IFACE=eth0
```
The object is always recompiled, so the values can't go stale. A BPF program **can't read the env at
runtime** (it runs in the kernel), so these are baked in at build — changing them means rebuild + reload.
To change them *without* reloading you'd add a config BPF map read per-packet + a `bpftool map update`
step; ask if you want that.
- `GAME_PORT` — must match the server's `-addr` port.
- `RATE_MAX` — packets/sec per source before dropping. **Lower = more aggressive** (catches moderate
  multi-source floods) but risks dropping a legit client's burst; **higher = only heavy floods**. Keep it
  comfortably above the ~35/s a normal client sends.

## Verify it's working under attack
```bash
sudo make dump                         # sources near RATE_MAX are being throttled
ethtool -S eth0 | grep -i xdp          # some drivers report rx_xdp_drop
# CPU should stay low: top -> the NIC softirq core is no longer pinned
```

## Limits (be honest about these)
- **Per-source** limiter. It devastates sources that individually flood. If the attack is *many* sources
  each staying *under* `RATE_MAX`, lower `RATE_MAX`, or move to a **whitelist**: only pass sources the app
  has authenticated (the Go server writes verified source IPs into a shared BPF map; XDP passes those and
  rate-limits everyone else hard). That's the robust answer for spoofed/distributed floods — ask and it
  can be wired up (needs a pinned map + the `cilium/ebpf` Go bindings).
- XDP saves **CPU**, not **bandwidth**. If the flood saturates your NIC packet-rate or uplink, it must be
  scrubbed upstream (provider DDoS protection). XDP only helps while you still have RX headroom.
- IPv6 is passed (not rate-limited). Add an IPv6 branch if your clients use it.
