#!/usr/bin/env bash
# One-shot deployment for a fresh Linux VPS with Docker + compose v2.
# Run from anywhere: ./deploy/deploy.sh   (optionally: ./deploy/deploy.sh host80)
#
# Optional env:
#   COMPOSE_OVERRIDES="docker-compose.host-80.yml"
#     extra -f overrides appended after the base (and host-80) file
#   BUILD_ARGS="GOPROXY=https://goproxy.cn,direct NPM_CONFIG_REGISTRY=https://registry.npmmirror.com"
#     --build-arg KEY=VALUE pairs for the image builds (e.g. CN mirrors)
#   MINIO=1
#     start the optional MinIO object storage service (profile "minio")
#   SERIAL=1
#     build the images one at a time instead of in parallel (recommended
#     on small VPSes: concurrent go+node builds can exhaust 2GB RAM and
#     hang the whole box in swap)
set -euo pipefail
cd "$(dirname "$0")/.."

COMPOSE=(docker compose -f deploy/docker-compose.yml)
[ "${1:-}" = "host80" ] && COMPOSE+=( -f deploy/docker-compose.host-80.yml )
for f in ${COMPOSE_OVERRIDES:-}; do COMPOSE+=( -f "deploy/$f" ); done
[ "${MINIO:-}" = "1" ] && COMPOSE+=( --profile minio )

BUILD_ARGS=()
for kv in ${BUILD_ARGS:-}; do BUILD_ARGS+=( --build-arg "$kv" ); done

if [ "${SERIAL:-}" = "1" ]; then
  for svc in app web; do
    "${COMPOSE[@]}" build ${BUILD_ARGS[@]+"${BUILD_ARGS[@]}"} "$svc"
  done
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker not found. Install it first, e.g.:"
  echo "  curl -fsSL https://get.docker.com | sh"
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose v2 plugin not found (install docker-compose-plugin)."
  exit 1
fi

if [ ! -f deploy/.env ]; then
  cp deploy/.env.example deploy/.env
  echo "Created deploy/.env — EDIT IT (MASTER_KEY / PAY_PUBLIC_URL) before re-running."
  exit 1
fi
if grep -q "change-me-to-32-random-chars" deploy/.env; then
  echo "Edit MASTER_KEY in deploy/.env first (e.g. openssl rand -hex 32)."
  exit 1
fi

"${COMPOSE[@]}" up -d --build ${BUILD_ARGS[@]+"${BUILD_ARGS[@]}"}
"${COMPOSE[@]}" ps
echo
echo "Health check (API):"
curl -sf --max-time 15 http://127.0.0.1:8080/healthz && echo
echo "Health check (web front):"
if [ "${1:-}" = "host80" ]; then curl -sf --max-time 15 http://127.0.0.1:8000/healthz && echo; else curl -sf --max-time 15 http://127.0.0.1/healthz && echo; fi
