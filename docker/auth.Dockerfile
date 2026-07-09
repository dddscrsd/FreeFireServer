# Auth server (Discord-OAuth "fbconnect"). ONE image; compose runs it two ways
# (connect-server / login-server) via the npm-workspace start scripts. Build
# context = repo root. Served over plain HTTP — the auth domains are
# Cloudflare-proxied, so CF terminates TLS and talks HTTP to the origin.
FROM node:20-slim
WORKDIR /app
ENV NODE_ENV=production
COPY auth/ ./
RUN npm ci --omit=dev
# Overridden per service in compose (start:connect / start:login).
CMD ["npm", "run", "start:connect"]
