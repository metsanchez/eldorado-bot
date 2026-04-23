#!/bin/bash
# Start the Eldorado bot, killing any existing instances first
cd "$(dirname "$0")"

# VPS / headless Linux: sanal X sunucusu (apt install xvfb). Masaüstünde DISPLAY doluysa atlanır.
XVFB_PREFIX=()
if [ "$(uname -s)" = "Linux" ] && command -v xvfb-run >/dev/null 2>&1; then
  if [ -z "${DISPLAY:-}" ] || [ "${ELDORADO_XVFB:-}" = "1" ]; then
    XVFB_PREFIX=(xvfb-run -a)
  fi
fi

MYPID=$$

echo "Killing existing bot instances..."
# Kill compiled bot binaries (exclude this script's PID)
pgrep -f 'eldorado-bot' | while read pid; do
  [ "$pid" != "$MYPID" ] && [ "$pid" != "$PPID" ] && kill -9 "$pid" 2>/dev/null
done
pgrep -f 'go-build.*bot' | while read pid; do
  [ "$pid" != "$MYPID" ] && [ "$pid" != "$PPID" ] && kill -9 "$pid" 2>/dev/null
done
pgrep -f 'go run.*cmd/bot' | while read pid; do
  [ "$pid" != "$MYPID" ] && [ "$pid" != "$PPID" ] && kill -9 "$pid" 2>/dev/null
done
pkill -9 -f "browser_login.py" 2>/dev/null
pkill -9 -f "send_chat_message.py" 2>/dev/null
pkill -9 -f "chat_server.py" 2>/dev/null
pkill -9 -f "patchright/driver" 2>/dev/null
pkill -9 -f "playwright_chromiumdev" 2>/dev/null
sleep 2

echo "Building..."
# -buildvcs=false: root başka kullanıcıya ait .git veya safe.directory yoksa VCS damgası exit 128 verir
go build -buildvcs=false -o eldorado-bot ./cmd/bot/ || exit 1

echo "Starting chat server (persistent browser)..."
"${XVFB_PREFIX[@]}" python3 scripts/chat_server.py &
CHAT_SERVER_PID=$!
cleanup_chat_server() { kill $CHAT_SERVER_PID 2>/dev/null || true; }
trap cleanup_chat_server EXIT
sleep 3
if kill -0 $CHAT_SERVER_PID 2>/dev/null; then
  echo "Chat server started (PID $CHAT_SERVER_PID)"
else
  echo "WARNING: Chat server failed to start, will use script fallback per message"
fi

echo "Starting bot..."
"${XVFB_PREFIX[@]}" ./eldorado-bot
