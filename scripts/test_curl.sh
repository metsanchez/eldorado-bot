#!/bin/bash
# Eldorado Bot - curl ile API testleri
# Kullanım: ./scripts/test_curl.sh

cd "$(dirname "$0")/.."

# .env'den değişkenleri oku
if [ -f .env ]; then
  TELEGRAM_BOT_TOKEN=$(grep '^TELEGRAM_BOT_TOKEN=' .env | cut -d= -f2-)
  TELEGRAM_CHAT_ID=$(grep '^TELEGRAM_CHAT_ID=' .env | cut -d= -f2-)
  ELDORADO_BASE_URL=$(grep '^ELDORADO_BASE_URL=' .env | cut -d= -f2-)
  ELDORADO_COOKIES=$(grep '^ELDORADO_COOKIES=' .env | cut -d= -f2-)
  TALKJS_TOKEN=$(grep '^TALKJS_TOKEN=' .env | cut -d= -f2-)
  TALKJS_NYM_ID=$(grep '^TALKJS_NYM_ID=' .env | cut -d= -f2-)
fi
TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN:-}
TELEGRAM_CHAT_ID=${TELEGRAM_CHAT_ID:-0}
ELDORADO_BASE_URL=${ELDORADO_BASE_URL:-https://www.eldorado.gg}

echo "=== 1. Telegram API (getMe) ==="
curl -s "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getMe" | python3 -m json.tool 2>/dev/null || curl -s "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/getMe"
echo ""

echo "=== 2. Telegram - Test mesajı gönder ==="
curl -s -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
  -H "Content-Type: application/json" \
  -d '{"chat_id":'"${TELEGRAM_CHAT_ID}"',"text":"🔧 Eldorado Bot curl test - '"$(date '+%H:%M:%S')"'","parse_mode":"HTML"}' | python3 -m json.tool 2>/dev/null || echo "OK"
echo ""

echo "=== 3. Eldorado site erişimi ==="
HTTP=$(curl -sL -o /dev/null -w "%{http_code}" "${ELDORADO_BASE_URL}")
echo "HTTP $HTTP"
echo ""

if [ -n "${ELDORADO_COOKIES}" ]; then
  echo "=== 4. Eldorado API (cookies ile) ==="
  curl -sL --max-time 15 \
    -H "Accept: application/json" \
    -H "User-Agent: Mozilla/5.0" \
    -H "Origin: ${ELDORADO_BASE_URL}" \
    -b "${ELDORADO_COOKIES}" \
    "${ELDORADO_BASE_URL}/api/boostingOffers/me/boostingSubscriptions" | head -c 500
  echo ""
else
  echo "=== 4. Eldorado API - Atlanıyor (ELDORADO_COOKIES boş) ==="
fi

echo ""
echo "=== 5. Dosya kontrolleri ==="
[ -f storage.json ] && echo "storage.json: OK ($(wc -c < storage.json) bytes)" || echo "storage.json: yok"
[ -f Radifix.jpeg ] && echo "Radifix.jpeg: OK" || echo "Radifix.jpeg: yok"
[ -f storage/browser_cookies.json ] && echo "browser_cookies.json: OK" || echo "browser_cookies.json: yok"
[ -f storage/talkjs_token.json ] && echo "talkjs_token.json: OK" || echo "talkjs_token.json: yok"

echo ""
if [ -n "${TALKJS_TOKEN}" ] && [ -n "${TALKJS_NYM_ID}" ]; then
  echo "=== 6. TalkJS token durumu ==="
  echo "TALKJS_TOKEN: ${#TALKJS_TOKEN} karakter"
  echo "TALKJS_NYM_ID: ${TALKJS_NYM_ID}"
  echo "(TalkJS say testi için conversation ID gerekir)"
else
  echo "=== 6. TalkJS - Atlanıyor (token veya nymId yok) ==="
fi

echo ""
echo "=== Test tamamlandı ==="
