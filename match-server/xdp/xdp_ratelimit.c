// SPDX-License-Identifier: GPL-2.0
// xdp_ratelimit.c — per-source-IP UDP rate limiter for the match server's game port.
//
// Attached to the NIC with XDP, this drops datagrams from any source IPv4 that exceeds RATE_MAX packets
// per RATE_WINDOW_NS to GAME_PORT — in the driver, BEFORE the kernel network stack. Under a multi-source
// flood each dropped packet costs almost nothing: no skb allocation, no conntrack, no UDP delivery, and
// (crucially) no ICMP port-unreachable reply, so it doesn't fill your egress either. Everything else —
// other ports (SSH), non-UDP, IPv6, IP-options packets — always passes.
//
// Tune GAME_PORT / RATE_MAX below and recompile. Build + load: see the Makefile / README.md.
// NOTE: this rate-limits PER SOURCE IP. It shreds sources that individually flood; a truly distributed
// attack where each of very many sources stays UNDER RATE_MAX needs a whitelist (only pass sources the
// app has authenticated) or upstream scrubbing — see README.md.

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/udp.h>
#include <linux/in.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

/* GAME_PORT / RATE_MAX are overridable at build time via -D (the Makefile passes them from the env, e.g.
 * `GAME_PORT=12345 make`). A BPF program can't read the process env at runtime — it runs in the kernel —
 * so these are baked in at compile time; changing them means rebuild + reload. See the Makefile / README. */
#ifndef GAME_PORT
#define GAME_PORT      10100         /* the UDP port the match server listens on */
#endif
#ifndef RATE_MAX
#define RATE_MAX       100           /* packets/window per source before dropping (a legit client is ~35/s) */
#endif
#define RATE_WINDOW_NS 1000000000ULL /* 1 second window */

struct src_rate {
	__u64 window_start; /* ktime of the current window */
	__u64 count;        /* packets counted this window (updated atomically) */
};

/* LRU hash: bounded memory. When full the kernel evicts the least-recently-used source, so an attack
 * spraying millions of source IPs can't grow the map without bound. */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__type(key, __u32); /* source IPv4 address, network byte order */
	__type(value, struct src_rate);
	__uint(max_entries, 1048576); /* up to ~1M tracked sources */
} src_rates SEC(".maps");

SEC("xdp")
int xdp_ratelimit(struct xdp_md *ctx)
{
	void *data = (void *)(long)ctx->data;
	void *data_end = (void *)(long)ctx->data_end;

	struct ethhdr *eth = data;
	if ((void *)(eth + 1) > data_end)
		return XDP_PASS;
	if (eth->h_proto != bpf_htons(ETH_P_IP))
		return XDP_PASS; /* not IPv4 (IPv6/ARP/...) -> pass */

	struct iphdr *ip = (void *)(eth + 1);
	if ((void *)(ip + 1) > data_end)
		return XDP_PASS;
	if (ip->protocol != IPPROTO_UDP)
		return XDP_PASS; /* not UDP (TCP/SSH/ICMP) -> pass */
	if (ip->ihl != 5)
		return XDP_PASS; /* IP options present (rare) -> pass, don't parse a variable offset */

	struct udphdr *udp = (void *)(ip + 1);
	if ((void *)(udp + 1) > data_end)
		return XDP_PASS;
	if (udp->dest != bpf_htons(GAME_PORT))
		return XDP_PASS; /* only rate-limit the game port; SSH etc. untouched */

	__u32 src = ip->saddr;
	__u64 now = bpf_ktime_get_ns();

	struct src_rate *r = bpf_map_lookup_elem(&src_rates, &src);
	if (!r) {
		struct src_rate init = { .window_start = now, .count = 1 };
		bpf_map_update_elem(&src_rates, &src, &init, BPF_ANY);
		return XDP_PASS; /* first packet from this source */
	}
	if (now - r->window_start >= RATE_WINDOW_NS) {
		/* window rolled over -> reset. Racy across cores under a flood, which only loosens the limit a
		 * hair at a window boundary — fine for rate limiting. */
		r->window_start = now;
		r->count = 1;
		return XDP_PASS;
	}
	__u64 n = __sync_fetch_and_add(&r->count, 1) + 1; /* atomic: concurrent cores can't undercount */
	if (n > RATE_MAX)
		return XDP_DROP; /* over the per-source limit -> drop in the driver */
	return XDP_PASS;
}

char _license[] SEC("license") = "GPL";
