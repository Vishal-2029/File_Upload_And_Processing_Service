#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:3000}"
EMAIL="seed@example.com"
PASSWORD="seed1234"

echo "==> Registering user..."
REGISTER=$(curl -s -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
echo "$REGISTER" | python3 -m json.tool 2>/dev/null || echo "$REGISTER"

echo ""
echo "==> Logging in..."
LOGIN=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
TOKEN=$(echo "$LOGIN" | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])" 2>/dev/null || echo "")

if [ -z "$TOKEN" ]; then
  echo "ERROR: Could not extract token. Response: $LOGIN"
  exit 1
fi
echo "Token: ${TOKEN:0:20}..."

echo ""
echo "==> Uploading sample PDF (if available)..."
if [ -f "testdata/sample.pdf" ]; then
  UPLOAD=$(curl -s -X POST "$BASE_URL/upload" \
    -H "Authorization: Bearer $TOKEN" \
    -F "file=@testdata/sample.pdf")
  echo "$UPLOAD" | python3 -m json.tool 2>/dev/null || echo "$UPLOAD"
  FILE_ID=$(echo "$UPLOAD" | python3 -c "import sys,json; print(json.load(sys.stdin)['file_id'])" 2>/dev/null || echo "")

  if [ -n "$FILE_ID" ]; then
    echo ""
    echo "==> Polling file status (3 attempts)..."
    for i in 1 2 3; do
      sleep 2
      STATUS=$(curl -s "$BASE_URL/files/$FILE_ID" \
        -H "Authorization: Bearer $TOKEN")
      echo "Attempt $i: $(echo "$STATUS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('status','?'))" 2>/dev/null)"
    done
  fi
else
  echo "No testdata/sample.pdf found, skipping upload test."
fi

echo ""
echo "==> Listing files..."
curl -s "$BASE_URL/files" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool 2>/dev/null

echo ""
echo "==> Seed complete."
echo "    MailHog UI:   http://localhost:8025"
echo "    MinIO Console: http://localhost:9001 (minioadmin / minioadmin)"
