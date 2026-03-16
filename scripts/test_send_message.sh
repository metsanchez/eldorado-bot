#!/bin/bash
# Mesaj gönderme testi - send_chat_message.py ile
# Kullanım: ./scripts/test_send_message.sh [request_id]
# request_id verilmezse storage.json'dan offer_pending bir talep seçilir

cd "$(dirname "$0")/.."

MSG='Hey! 👋 Radiant Top #2 player here.


I personally handle every boost with the highest win rates and fastest completion on the platform.


🏆 100% Win Rate Record
⚡ Lightning-Fast Delivery
🔒 Full Account Security (VPN + Offline Mode)


✅ Free Live Stream
✅ Free Agent Selection
✅ Free Priority Queue


I treat every account like my own. Let'\''s get you to your dream rank — fast, safe, and guaranteed. 💎'

REQUEST_ID="$1"
if [ -z "$REQUEST_ID" ]; then
  REQUEST_ID=$(python3 -c "
import json
try:
    with open('storage.json') as f:
        d = json.load(f)
    for oid, o in d.get('trackedOrders', {}).items():
        if o.get('trackingStatus') == 'offer_pending':
            print(oid)
            break
except Exception:
    pass
" 2>/dev/null)
fi

if [ -z "$REQUEST_ID" ]; then
  echo "Kullanım: $0 <request_id>"
  echo "veya storage.json'da offer_pending talep olmalı"
  exit 1
fi

IMAGE="Radifix.jpeg"
[ ! -f "$IMAGE" ] && IMAGE=""

echo "=== Mesaj gönderme testi ==="
echo "Request ID: $REQUEST_ID"
echo "Görsel: ${IMAGE:-yok}"
echo ""

if [ -n "$IMAGE" ]; then
  python3 scripts/send_chat_message.py "$REQUEST_ID" "$MSG" "$IMAGE" 2>&1
else
  python3 scripts/send_chat_message.py "$REQUEST_ID" "$MSG" 2>&1
fi

echo ""
echo "=== Test tamamlandı ==="
