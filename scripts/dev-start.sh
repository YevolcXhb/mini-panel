#!/bin/bash

# Quick start script for development

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "Starting Mini Panel in development mode..."

# Start backend
cd "${PROJECT_DIR}/backend"
echo "[Backend] Starting Go server..."
go run cmd/server/main.go &
BACKEND_PID=$!

# Start frontend
cd "${PROJECT_DIR}/frontend"
echo "[Frontend] Starting Vite dev server..."
npm run dev &
FRONTEND_PID=$!

echo ""
echo "Backend PID: ${BACKEND_PID}"
echo "Frontend PID: ${FRONTEND_PID}"
echo ""
echo "Backend:  http://localhost:8888"
echo "Frontend: http://localhost:3000"
echo ""
echo "Press Ctrl+C to stop both"

wait
