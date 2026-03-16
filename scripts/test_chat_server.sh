#!/bin/bash
# Chat server test - önce server'ı başlat, sonra bu script ile test et
# 1. python3 scripts/chat_server.py &   (ayrı terminalde)
# 2. ./scripts/test_chat_server.sh <request_id>

cd "$(dirname "$0")/.."

REQUEST_ID="${1:-}"
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
  exit 1
fi

echo "=== Chat server test ==="
echo "Request ID: $REQUEST_ID"
echo ""

python3 -c "
import json, os, urllib.request

request_id = '$REQUEST_ID'
msg = 'Test mesaj - chat server'
d = {'request_id': request_id, 'message': msg}
img = 'Radifix.jpeg'
if os.path.isfile(img):
    d['image_path'] = os.path.abspath(img)

req = urllib.request.Request('http://127.0.0.1:38521/send',
    data=json.dumps(d).encode(),
    headers={'Content-Type': 'application/json'},
    method='POST')
try:
    with urllib.request.urlopen(req, timeout=90) as r:
        print(json.dumps(json.load(r), indent=2))
except Exception as e:
    print('Error:', e)
"

echo ""
echo "=== Test tamamlandı ==="
