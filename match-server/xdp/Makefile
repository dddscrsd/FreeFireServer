# Build + load the XDP per-source rate limiter for the match server.
#
#   make                                 # compile xdp_ratelimit.o
#   sudo make load   IFACE=eth0          # attach in native (driver) mode
#   sudo make load   IFACE=eth0 MODE=skb # attach in generic/skb mode (works on ANY driver)
#   sudo make unload IFACE=eth0
#   sudo make status IFACE=eth0
#   sudo make dump                       # dump the per-source counters
#
# Requires: clang + llvm, kernel BPF headers (libbpf-dev / kernel-headers), and iproute2 (`ip`).
#   Debian/Ubuntu: sudo apt install clang llvm libbpf-dev linux-headers-$(uname -r) iproute2
#   RHEL/Fedora:   sudo dnf install clang llvm libbpf-devel kernel-headers iproute

CLANG     ?= clang
IFACE     ?= eth0
MODE      ?= native        # native | skb | hw
GAME_PORT ?= 10100         # must match the server's -addr port; from the env: GAME_PORT=12345 make load
RATE_MAX  ?= 100           # per-source packets/sec before XDP_DROP
OBJ    = xdp_ratelimit.o
SRC    = xdp_ratelimit.c
SEC    = xdp

# ip link's mode flag: native driver=xdpdrv, generic/skb=xdpgeneric, hardware offload=xdpoffload.
XDPMODE = $(if $(filter skb generic,$(MODE)),xdpgeneric,$(if $(filter hw,$(MODE)),xdpoffload,xdpdrv))

.PHONY: all load unload status dump clean FORCE

all: $(OBJ)

# Depend on FORCE so the object is always recompiled — GAME_PORT/RATE_MAX are -D build params from the
# env, and Make wouldn't otherwise notice they changed while the .c is untouched (stale baked-in port).
# The BPF compile is trivial, so always rebuilding is cheap and avoids a confusing wrong-port load.
$(OBJ): $(SRC) FORCE
	$(CLANG) -O2 -g -Wall -DGAME_PORT=$(GAME_PORT) -DRATE_MAX=$(RATE_MAX) -target bpf -c $(SRC) -o $(OBJ)

FORCE:

load: $(OBJ)
	ip link set dev $(IFACE) $(XDPMODE) obj $(OBJ) sec $(SEC)
	@echo "attached $(OBJ) to $(IFACE) ($(XDPMODE)). unload with: sudo make unload IFACE=$(IFACE)"

unload:
	ip link set dev $(IFACE) xdp off
	@echo "detached XDP from $(IFACE)"

status:
	@ip link show dev $(IFACE) | grep -o 'xdp[a-z]*' || echo "no XDP program attached to $(IFACE)"

dump:
	@bpftool map dump name src_rates 2>/dev/null | head -60 || echo "map not loaded (or bpftool missing)"

clean:
	rm -f $(OBJ)
