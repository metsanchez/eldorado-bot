#!/usr/bin/env python3
"""
Send a chat message to a buyer on Eldorado using patchright browser.
Reads saved cookies from storage/browser_cookies.json.

Usage:
  send_chat_message.py <boosting_request_id> <message_text> [image_path] [talkjs_conversation_id]
"""

import json
import os
import sys

from patchright.sync_api import sync_playwright

from chat_ops import (
    COOKIES_FILE,
    load_cookies,
    log,
    send_chat_message_impl,
)


def main():
    if len(sys.argv) < 3:
        print(json.dumps({"error": "Usage: send_chat_message.py <request_id> <message> [image_path] [conversation_id]"}))
        sys.exit(1)

    request_id = sys.argv[1]
    message_text = sys.argv[2]
    image_path = sys.argv[3] if len(sys.argv) > 3 and sys.argv[3] else None
    conversation_id = sys.argv[4] if len(sys.argv) > 4 and sys.argv[4] else None

    if image_path and not os.path.isfile(image_path):
        log(f"WARNING: image file not found: {image_path}")
        image_path = None

    cookies = load_cookies()
    if not cookies:
        print(json.dumps({"error": "cookies file not found, login first"}))
        sys.exit(1)

    log(f"loaded {len(cookies)} cookies from file")

    headless = os.environ.get("HEADLESS", "").lower() in ("1", "true", "yes")
    base_url = os.environ.get("ELDORADO_BASE_URL", "https://www.eldorado.gg").rstrip("/")

    with sync_playwright() as p:
        browser = p.chromium.launch(
            channel="chrome",
            headless=headless,
            args=[
                "--disable-blink-features=AutomationControlled",
                "--no-sandbox",
                "--disable-dev-shm-usage",
                "--disable-gpu",
                "--disable-software-rasterizer",
                "--disable-extensions",
                "--no-first-run",
            ],
        )
        context = browser.new_context(no_viewport=True)
        context.add_cookies(cookies)
        log(f"loaded {len(cookies)} cookies into browser")

        page = context.new_page()

        success, route, err = send_chat_message_impl(
            page, request_id, message_text, image_path, conversation_id, base_url
        )
        browser.close()

        if not success:
            print(json.dumps({"error": err or "unknown error"}))
            sys.exit(1)

        print(json.dumps({"success": True, "request_id": request_id, "route": route}))


if __name__ == "__main__":
    main()
