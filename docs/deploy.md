# Deployment & edge (Phase 0b)

The stack runs as containers behind a Traefik edge proxy. GitHub Actions builds a
per-module image, pushes to GHCR, and the VPS pulls + restarts.

## Topology

```
  DNS *.example.com ─▶ Traefik ─┬─ live.  → live    (:3000)
                                ├─ login. → login   (:3001)   L7, auto-TLS
                                ├─ main.  → main    (:3002)
                                └─ :10300 → gateway (:10300)  L4 TCP passthrough
  match (:10100/udp) ── addressed directly by the client, allocated by the matchmaker
  backing: postgres, redis  (private)
```

The four Node services share one image (`ff-node`) and differ only by command +
Traefik labels, so each scales on its own (`docker compose up -d --scale main=3`).
The match-server is a separate image (`ff-match`) and is **not** proxied.

## VPS one-time setup

1. Install Docker Engine + the compose plugin.
2. Point DNS `A` records for each domain at the VPS; open the firewall / OCI
   security list for `80`, `443`, `10300/tcp`, and `10100/udp`.
3. Put the deploy files on the box:
   ```sh
   sudo mkdir -p /opt/freefire && cd /opt/freefire
   # copy docker-compose.yml here, plus a .env (from .env.example) with real
   # domains, POSTGRES_PASSWORD, MATCH_JWT_SECRET, ACME_EMAIL, NODE_ID.
   docker login ghcr.io          # or make the GHCR packages public
   ```
4. Add the GitHub Actions secrets `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY`.

## Deploy

Push to `main` (or `feat/infra-migration`) → the workflow builds `ff-node` +
`ff-match`, pushes `:latest` and `:<sha>` to GHCR, then SSHes in and runs
`docker compose pull && up -d`. HTTP/gateway roll one replica at a time; the
migration gate (Phase 1+) and match-drain (Phase 3+) are marked as TODO in the
deploy step.

## Local

```sh
docker compose up --build        # builds both images from source, brings up the stack
```

Set the domains in `.env` to something resolvable (or add hosts entries) for
Traefik routing; for pure bus/DB work you can `docker compose up -d redis postgres`
and run the servers on the host.
