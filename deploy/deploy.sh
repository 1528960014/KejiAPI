#!/usr/bin/env bash
# One-shot deployment for a fresh Linux VPS with Docker + compose v2.
# Run from anywhere: ./deploy/deploy.sh   (optionally: ./deploy/deploy.sh host80)
set -euo pipefail
cd "$(dirname "$0")/.."

COMPOSE=(docker compose -f deploy/docker-compose.yml)
[ "${1:-}" = "host80" ] && COMPOSE+=( -f deploy/docker-compose.host-80.yml )

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

"${COMPOSE[@]}" up -d --build
"${COMPOSE[@]}" ps
echo
echo "Health check (API):"
curl -sf --max-time 15 http://127.0.0.1:8080/healthz && echo
echo "Health check (web front):"
if [ "${1:-}" = "host80" ]; then curl -sf --max-time 15 http://127.0.0.1:8000/healthz && echo; else curl -sf --max-time 15 http://127.0.0.1/healthz && echo; fi
