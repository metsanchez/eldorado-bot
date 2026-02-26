#!/usr/bin/env python3
"""
Send a chat message to a buyer on Eldorado using patchright browser.
Reads saved cookies from storage/browser_cookies.json.

Flow:
  1. Navigate to /boosting-request/{requestId}
  2. Click "Chat with buyer" button
  3. Find TalkJS iframe, locate the "Say something..." textarea
  4. Type and send the message

Usage:
  send_chat_message.py <boosting_request_id> <message_text> [image_path]
"""

import json
import os
import sys
import time

from patchright.sync_api import sync_playwright

COOKIES_FILE = os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "..", "storage", "browser_cookies.json"
)


def main():
    if len(sys.argv) < 3:
        print(json.dumps({"error": "Usage: send_chat_message.py <request_id> <message> [image_path]"}))
        sys.exit(1)

    request_id = sys.argv[1]
    message_text = sys.argv[2]
    image_path = sys.argv[3] if len(sys.argv) > 3 else None

    if image_path and not os.path.isfile(image_path):
        log(f"WARNING: image file not found: {image_path}")
        image_path = None

    if not os.path.isfile(COOKIES_FILE):
        print(json.dumps({"error": "cookies file not found, login first"}))
        sys.exit(1)

    with open(COOKIES_FILE) as f:
        cookies = json.load(f)
    log(f"loaded {len(cookies)} cookies from file")

    with sync_playwright() as p:
        browser = p.chromium.launch(
            channel="chrome",
            headless=False,
            args=["--disable-blink-features=AutomationControlled", "--no-sandbox"],
        )
        context = browser.new_context(no_viewport=True)

        valid_cookies = []
        for c in cookies:
            cookie = {
                "name": c["name"],
                "value": c["value"],
                "domain": c.get("domain", ".eldorado.gg"),
                "path": c.get("path", "/"),
            }
            if c.get("expires", -1) > 0:
                cookie["expires"] = c["expires"]
            if c.get("secure"):
                cookie["secure"] = True
            if c.get("httpOnly"):
                cookie["httpOnly"] = True
            if c.get("sameSite"):
                cookie["sameSite"] = c["sameSite"]
            valid_cookies.append(cookie)

        context.add_cookies(valid_cookies)
        log(f"loaded {len(valid_cookies)} cookies into browser")

        page = context.new_page()

        url = f"https://www.eldorado.gg/boosting-request/{request_id}"
        log(f"navigating to {url}")
        page.goto(url, wait_until="domcontentloaded", timeout=30000)

        wait_for_cloudflare(page, "request-detail")

        page.wait_for_timeout(5000)
        log(f"page title: {page.title()}")
        log(f"current URL: {page.url[:100]}")

        # Wait for Angular SPA to render the page content
        chat_btn = None
        for attempt in range(15):
            for selector in [
                'button:has-text("Chat with buyer")',
                'text="Chat with buyer"',
                'button:has-text("Chat")',
            ]:
                try:
                    el = page.query_selector(selector)
                    if el and el.is_visible():
                        chat_btn = el
                        log(f"found chat button: {selector}")
                        break
                except Exception:
                    continue
            if chat_btn:
                break
            log(f"waiting for chat button... ({attempt + 1}/15)")
            page.wait_for_timeout(2000)

        if not chat_btn:
            buttons = page.query_selector_all("button")
            btn_texts = []
            for b in buttons[:20]:
                try:
                    txt = b.inner_text()
                    if txt.strip():
                        btn_texts.append(txt.strip()[:50])
                except Exception:
                    pass
            log(f"ERROR: 'Chat with buyer' not found. Buttons on page: {btn_texts}")
            print(json.dumps({"error": "Chat with buyer button not found", "url": page.url[:200], "buttons": btn_texts}))
            browser.close()
            sys.exit(1)

        chat_btn.click()
        log("clicked 'Chat with buyer'")
        page.wait_for_timeout(4000)

        # Find TalkJS iframe and the chat input inside it
        talkjs_frame = find_talkjs_frame(page)
        if not talkjs_frame:
            log("ERROR: TalkJS iframe not found")
            print(json.dumps({"error": "TalkJS chat iframe not found"}))
            browser.close()
            sys.exit(1)

        # Send image first if provided
        if image_path:
            send_image(talkjs_frame, image_path, page)

        # Send text message
        success = send_message(talkjs_frame, message_text)
        if not success:
            print(json.dumps({"error": "could not send message - input not found"}))
            browser.close()
            sys.exit(1)

        page.wait_for_timeout(2000)
        log("message sent successfully")
        browser.close()
        print(json.dumps({"success": True, "request_id": request_id}))


def find_talkjs_frame(page):
    """Find the TalkJS chat iframe."""
    for attempt in range(10):
        for frame in page.frames:
            frame_url = frame.url.lower()
            if "talkjs" in frame_url or "chatbox" in frame_url:
                log(f"found TalkJS frame: {frame.url[:80]}")
                return frame
        log(f"waiting for TalkJS frame... ({attempt + 1}/10)")
        page.wait_for_timeout(1000)

    log("TalkJS frame not found after 10 attempts")
    return None


def send_message(frame, text):
    """Type and send a message in the TalkJS chat."""
    chat_input = None
    for selector in [
        '[placeholder="Say something..."]',
        'textarea',
        '[role="textbox"]',
        'div[contenteditable="true"]',
        '[class*="message-input"]',
    ]:
        try:
            el = frame.query_selector(selector)
            if el:
                chat_input = el
                log(f"found chat input: {selector}")
                break
        except Exception:
            continue

    if not chat_input:
        log("ERROR: chat input not found in TalkJS frame")
        return False

    chat_input.click()
    frame.wait_for_timeout(300)

    # Type message line by line using Shift+Enter for newlines, then Enter to send
    lines = text.split("\n")
    for i, line in enumerate(lines):
        if i > 0:
            chat_input.press("Shift+Enter")
            frame.wait_for_timeout(100)
        if line:
            chat_input.type(line)
            frame.wait_for_timeout(100)

    frame.wait_for_timeout(300)
    chat_input.press("Enter")
    frame.wait_for_timeout(1000)
    log("message typed and submitted")
    return True


def send_image(frame, image_path, page):
    """Upload an image to the TalkJS chat and click Send."""
    abs_path = os.path.abspath(image_path)
    log(f"uploading image: {abs_path}")

    for attempt in range(5):
        try:
            file_input = frame.query_selector('input[type="file"]')
            if file_input:
                file_input.set_input_files(abs_path)
                page.wait_for_timeout(3000)
                log("image selected via file input")

                # After selecting a file, TalkJS shows a preview with "Send" button
                send_clicked = False
                for sel in [
                    'button:has-text("Send")',
                    'button[type="submit"]',
                    '[data-testid="send-button"]',
                ]:
                    try:
                        # Check both iframe and main page for the Send button
                        btn = frame.query_selector(sel)
                        if btn and btn.is_visible():
                            btn.click()
                            send_clicked = True
                            log(f"clicked Send button in iframe: {sel}")
                            break
                    except Exception:
                        continue

                if not send_clicked:
                    # Send button might be on the main page (outside iframe)
                    for sel in [
                        'button:has-text("Send")',
                        'button[type="submit"]:visible',
                    ]:
                        try:
                            btn = page.query_selector(sel)
                            if btn and btn.is_visible():
                                btn.click()
                                send_clicked = True
                                log(f"clicked Send button on page: {sel}")
                                break
                        except Exception:
                            continue

                page.wait_for_timeout(3000)
                if send_clicked:
                    log("image sent successfully")
                else:
                    log("WARNING: Send button not found, image may not be sent")
                return True
        except Exception as e:
            log(f"image upload attempt {attempt + 1}: {e}")
        page.wait_for_timeout(2000)

    log("WARNING: could not upload image after retries")
    return False


def wait_for_cloudflare(page, page_name, timeout=60):
    """Wait for Cloudflare challenge to resolve."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            title = page.title().lower()
        except Exception:
            page.wait_for_timeout(3000)
            continue

        if not any(kw in title for kw in ["just a moment", "bir dakika", "cloudflare"]):
            return

        for frame in page.frames:
            if "challenges.cloudflare.com" in frame.url:
                try:
                    body = frame.query_selector("body")
                    if body:
                        body.click()
                except Exception:
                    pass
                break

        page.wait_for_timeout(2000)


def log(msg):
    print(f"[chat-msg] {msg}", file=sys.stderr)


if __name__ == "__main__":
    main()
