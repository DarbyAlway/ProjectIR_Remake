#!/usr/bin/env bash
# Start or restart all services.
# Usage: bash deploy/start.sh

set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "Starting Elasticsearch ..."
sudo docker compose -f "$ROOT/docker-compose.yml" up -d

echo "Enabling and starting services ..."
sudo systemctl enable --now projectir-recommender
sudo systemctl enable --now projectir-server

sudo systemctl restart projectir-recommender
sudo systemctl restart projectir-server

echo ""
echo "Status:"
sudo systemctl is-active projectir-recommender && echo "  recommender: running" || echo "  recommender: FAILED"
sudo systemctl is-active projectir-server      && echo "  server:      running" || echo "  server:      FAILED"

echo ""
echo "Logs:"
echo "  sudo journalctl -u projectir-server      -f"
echo "  sudo journalctl -u projectir-recommender -f"
