#!/usr/bin/env bash
# Usage:  ./dev.sh
# Stop :  Ctrl+C

ROOT="$(cd "$(dirname "$0")" && pwd)"

if [ -f "$ROOT/.env" ]; then
  export $(grep -v '^#' "$ROOT/.env" | grep -v '^Username' | grep -v '^Password' | xargs)
fi

if [ -z "$MONGODB_URI" ]; then
  echo "[error] MONGODB_URI is not set. Check your .env file."
  exit 1
fi

PYTHON="/home/patta/miniconda3/envs/project_IR/bin/python"
mkdir -p "$ROOT/logs"

PIDS=()
cleanup() {
  echo ""
  echo "Shutting down..."
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null
  done
  wait 2>/dev/null
  echo "All services stopped."
}
trap cleanup EXIT INT TERM

# ── 1. Recommender (Python) ───────────────────────────────────────────────────
echo "[1/3] Starting recommender service (port 5001)..."
"$PYTHON" "$ROOT/preprocess/recommender_service.py" > "$ROOT/logs/recommender.log" 2>&1 &
PIDS+=($!)

for i in $(seq 1 30); do
  if curl -sf http://localhost:5001/health > /dev/null 2>&1; then
    echo "      Recommender ready."
    break
  fi
  sleep 1
  if [ $i -eq 30 ]; then
    echo "      [warn] Recommender didn't start in 30 s — check logs/recommender.log"
  fi
done

# ── 2. Go server ──────────────────────────────────────────────────────────────
echo "[2/3] Starting Go server (port 8080)..."
cd "$ROOT/server"
go run main.go > "$ROOT/logs/server.log" 2>&1 &
PIDS+=($!)

for i in $(seq 1 20); do
  if nc -z localhost 8080 2>/dev/null; then
    echo "      Go server ready."
    break
  fi
  sleep 1
  if [ $i -eq 20 ]; then
    echo "      [warn] Go server didn't start in 20 s — check logs/server.log"
  fi
done

# ── 3. React frontend (foreground) ────────────────────────────────────────────
echo "[3/3] Starting React frontend (port 5173)..."
echo ""
echo "  Recommender : http://localhost:5001"
echo "  Go API      : http://localhost:8080"
echo "  App         : http://localhost:5173"
echo ""
cd "$ROOT/frontend"
npm run dev
