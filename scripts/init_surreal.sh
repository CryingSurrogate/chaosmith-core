#!/usr/bin/env bash
set -euo pipefail

SURREAL_URL=${SURREAL_URL:-http://127.0.0.1:8000}
SURREAL_USER=${SURREAL_USER:-root}
SURREAL_PASS=${SURREAL_PASS:-root}
SURREAL_NS=${SURREAL_NS:-chaosmith}
SURREAL_DB=${SURREAL_DB:-wims}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --url) SURREAL_URL="$2"; shift 2;;
    --user) SURREAL_USER="$2"; shift 2;;
    --pass) SURREAL_PASS="$2"; shift 2;;
    --ns) SURREAL_NS="$2"; shift 2;;
    --db) SURREAL_DB="$2"; shift 2;;
    *) echo "unknown flag: $1"; exit 1;;
  esac
done

curl -sS -u "$SURREAL_USER:$SURREAL_PASS" \
  -H "NS: $SURREAL_NS" -H "DB: $SURREAL_DB" \
  -H 'Content-Type: application/surrealql' \
  --data-binary @etc/schema.surql \
  "$SURREAL_URL/sql"
echo

echo "Schema applied to $SURREAL_URL (ns=$SURREAL_NS db=$SURREAL_DB)"
