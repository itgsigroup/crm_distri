# Distri ARC Orbit — production images (docs/DEPLOY.md).
#   docker build -f infra/Dockerfile --target arc -t distri-arc/arc .    # api + worker + ctl (one binary)
#   docker build -f infra/Dockerfile --target web -t distri-arc/web .    # Caddy serving web/dist + reverse proxy
ARG GO_VERSION=1.27
ARG NODE_VERSION=22

FROM golang:${GO_VERSION}-bookworm AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X distri-arc/internal/config.Version=${VERSION}" -o /out/arc ./cmd/arc

FROM node:${NODE_VERSION}-bookworm-slim AS webbuild
WORKDIR /src
COPY web/package.json web/package-lock.json web/
RUN cd web && npm ci
COPY web web
COPY reference reference
RUN cd web && npm run build

# api / worker / ctl: pg_dump + openssl for backups, tzdata for WIB, no shell tools beyond bash
FROM debian:bookworm-slim AS arc
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata openssl curl gnupg \
 && install -d /usr/share/postgresql-common/pgdg \
 && curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
 && echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt bookworm-pgdg main" > /etc/apt/sources.list.d/pgdg.list \
 && apt-get update && apt-get install -y --no-install-recommends postgresql-client-16 \
 && apt-get purge -y curl gnupg && apt-get autoremove -y && rm -rf /var/lib/apt/lists/* \
 && useradd --system --uid 10001 --home /app arc
WORKDIR /app
COPY --from=gobuild /out/arc /usr/local/bin/arc
COPY infra/backup.sh infra/restore.sh /usr/local/bin/
USER arc
ENV TZ=Asia/Jakarta APP_ENV=prod
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/usr/local/bin/arc", "ctl", "counts"]
ENTRYPOINT ["/usr/local/bin/arc"]
CMD ["api"]

FROM caddy:2-alpine AS web
COPY --from=webbuild /src/web/dist /srv/web
COPY infra/Caddyfile /etc/caddy/Caddyfile

# WhatsApp Baileys bridge (ADR 0017)
FROM node:${NODE_VERSION}-bookworm-slim AS bridgebuild
WORKDIR /src
COPY apps/wa-bridge/package.json apps/wa-bridge/package-lock.json ./
RUN npm ci
COPY apps/wa-bridge ./
RUN npm run build && npm prune --omit=dev

FROM node:${NODE_VERSION}-bookworm-slim AS wa-bridge
WORKDIR /app
COPY --from=bridgebuild /src/dist ./dist
COPY --from=bridgebuild /src/node_modules ./node_modules
COPY --from=bridgebuild /src/package.json ./
USER node
ENV NODE_ENV=production BRIDGE_HOST=0.0.0.0 BRIDGE_PORT=8111 BRIDGE_DATA_DIR=/data
CMD ["node", "dist/main.js"]
