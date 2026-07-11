# Node tier — ONE image; each compose service (live / login / main / gateway)
# runs a different entry point via its command, so they scale independently
# behind Traefik. Build context is the repo root.
FROM node:24-slim AS deps
WORKDIR /app
# better-sqlite3 is a native module; it needs a toolchain to compile.
RUN apt-get update \
  && apt-get install -y --no-install-recommends python3 make g++ \
  && rm -rf /var/lib/apt/lists/*
COPY package.json package-lock.json ./
RUN npm ci --omit=dev

FROM node:24-slim
WORKDIR /app
ENV NODE_ENV=production
COPY --from=deps /app/node_modules ./node_modules
COPY package.json ./
COPY config ./config
COPY proto ./proto
COPY protocol ./protocol
COPY migrations ./migrations
COPY scripts ./scripts
COPY src ./src
# Overridden per service in docker-compose (e.g. node src/servers/login.js).
CMD ["node", "src/servers/main.js"]
