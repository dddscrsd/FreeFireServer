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

CLANG ?= clang
IFACE ?= eth0
MODE  ?= native            # native | skb | hw
OBJ    = xdp_ratelimit.o
SRC    = xdp_ratelimit.c
SEC    = xdp

# ip link's mode flag: native driver=xdpdrv, generic/skb=xdpgeneric, hardware offload=xdpoffload.
XDPMODE = $(if $(filter skb generic,$(MODE)),xdpgeneric,$(if $(filter hw,$(MODE)),xdpoffload,xdpdrv))

.PHONY: all load unload status dump clean

all: $(OBJ)

$(OBJ): $(SRC)
	$(CLANG) -O2 -g -Wall -target bpf -c $(SRC) -o $(OBJ)

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
