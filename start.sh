#!/bin/bash
# Start the Eldorado bot, killing any existing instances first
cd "$(dirname "$0")"

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
pkill -9 -f "patchright/driver" 2>/dev/null
pkill -9 -f "playwright_chromiumdev" 2>/dev/null
sleep 2

echo "Building..."
go build -o eldorado-bot ./cmd/bot/ || exit 1

echo "Starting bot..."
exec ./eldorado-bot
