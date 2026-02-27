#!/bin/bash
# TalkJS say API test - curl ile mesaj gönder
# Kullanım: ./scripts/test_talkjs_curl.sh CONV_ID TOKEN [MESSAGE]
# Örnek: ./scripts/test_talkjs_curl.sh 2233ff99dc5b9ca92a1e 'eyJhbGciOiJIUzI1...' "hey"
# .env'den: source .env 2>/dev/null; ./scripts/test_talkjs_curl.sh 2233ff99dc5b9ca92a1e "$TALKJS_TOKEN"

CONV_ID="${1:-eda998110f359fd9dc1d}"
TOKEN="${2}"
MSG="${3:-hey}"
NYM_ID="${TALKJS_NYM_ID:-1ae50f717a66884f2184_n}"

if [[ -z "$TOKEN" ]]; then
  echo "Usage: $0 CONV_ID TOKEN [MESSAGE]"
  echo "Or: TALKJS_TOKEN=xxx $0 CONV_ID"
  exit 1
fi

IDEMPOTENCY="-$(openssl rand -base64 15 | tr '+/' '-_' | head -c 20)"
SESSION_ID=$(uuidgen 2>/dev/null || echo "c3535284-af69-4a1a-9cf6-30335e575106")

RESP=$(curl -sS -w "\n%{http_code}" "https://app.talkjs.com/api/v0/49mLECOW/say/${CONV_ID}/?sessionId=${SESSION_ID}" \
  --compressed \
  -X POST \
  -H "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0" \
  -H "Accept: application/json" \
  -H "Referer: https://app.talkjs.com/" \
  -H "Content-Type: application/json" \
  -H "x-talkjs-client-build: frontend-release-855acf7" \
  -H "x-talkjs-client-date: $(date -u +%Y-%m-%dT%H:%M:%S.000Z)" \
  -H "Authorization: bearer ${TOKEN}" \
  -H "Origin: https://app.talkjs.com" \
  -d "{\"idempotencyKey\":\"$IDEMPOTENCY\",\"entityTree\":[\"$MSG\"],\"received\":false,\"custom\":{},\"nymId\":\"$NYM_ID\",\"attachment\":null,\"location\":null}")

HTTP_CODE=$(echo "$RESP" | tail -n1)
BODY=$(echo "$RESP" | sed '$d')
echo "$BODY"
if [[ "$HTTP_CODE" == "200" ]] && echo "$BODY" | grep -q '"ok"'; then
  echo ""; echo "OK - mesaj gonderildi"
  exit 0
else
  echo ""; echo "HTTP $HTTP_CODE - hata olabilir (token/conv eski olabilir)"
  exit 1
fi
