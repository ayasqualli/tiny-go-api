#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:3000}"

echo '1) Health check'
curl -sS -i "$BASE_URL/health"

echo -e '\n\n2) List seed tasks'
curl -sS -i "$BASE_URL/tasks"

echo -e '\n\n3) Create a task'
created="$(curl -fsS -X POST "$BASE_URL/tasks" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Buy milk"}')"
echo "$created"
id="$(printf '%s' "$created" | sed -n 's/.*"id":\([0-9][0-9]*\).*/\1/p')"

echo -e '\n4) Update the task'
curl -sS -i -X PUT "$BASE_URL/tasks/$id" \
  -H 'Content-Type: application/json' \
  -d '{"title":"Buy oat milk","done":true}'

echo -e '\n\n5) Filter completed tasks'
curl -sS -i "$BASE_URL/tasks?done=true&search=milk"

echo -e '\n\n6) View statistics'
curl -sS -i "$BASE_URL/stats"

echo -e '\n\n7) Delete the task'
curl -sS -i -X DELETE "$BASE_URL/tasks/$id"

echo -e '\n\n8) Confirm deletion returns 404'
curl -sS -i "$BASE_URL/tasks/$id" || true
