#!/usr/bin/env python3
"""
Chat server: persistent browser, new tab per message, parallel support, tab cleanup.
HTTP API: POST /send with JSON {request_id, message, image_path?, conversation_id?}
"""

import json
import os
import signal
import sys
import threading
import time
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse

from patchright.sync_api import sync_playwright

from chat_ops import load_cookies, log, send_chat_message_impl

# Config via env
PORT = int(os.environ.get("CHAT_SERVER_PORT", "38521"))
MAX_CONCURRENT = int(os.environ.get("CHAT_SERVER_MAX_CONCURRENT", "5"))
TAB_MAX_AGE_SEC = int(os.environ.get("CHAT_SERVER_TAB_MAX_AGE_SEC", "300"))  # 5 min
CLEANUP_INTERVAL_SEC = int(os.environ.get("CHAT_SERVER_CLEANUP_INTERVAL_SEC", "60"))

# Global state
playwright = None
browser = None
context = None
semaphore = None
page_timestamps = {}  # page_id -> created_at
page_timestamps_lock = threading.Lock()
shutdown_requested = False


def get_browser_args():
    return [
        "--disable-blink-features=AutomationControlled",
        "--no-sandbox",
        "--disable-dev-shm-usage",
        "--disable-gpu",
        "--disable-software-rasterizer",
        "--disable-extensions",
        "--no-first-run",
    ]


def init_browser():
    global playwright, browser, context, semaphore
    headless = os.environ.get("HEADLESS", "").lower() in ("1", "true", "yes")
    playwright = sync_playwright().start()
    browser = playwright.chromium.launch(
        channel="chrome",
        headless=headless,
        args=get_browser_args(),
    )
    context = browser.new_context(no_viewport=True)

    cookies = load_cookies()
    if not cookies:
        log("ERROR: cookies file not found, login first")
        sys.exit(1)
    context.add_cookies(cookies)
    log(f"browser started, {len(cookies)} cookies loaded")

    semaphore = threading.Semaphore(MAX_CONCURRENT)
    log(f"max concurrent: {MAX_CONCURRENT}, tab max age: {TAB_MAX_AGE_SEC}s, cleanup every: {CLEANUP_INTERVAL_SEC}s")


def cleanup_old_tabs():
    """Close pages that have been open longer than TAB_MAX_AGE_SEC to prevent RAM bloat."""
    global context, page_timestamps
    if not context:
        return
    now = time.time()
    to_close = []
    with page_timestamps_lock:
        for page_id, created_at in list(page_timestamps.items()):
            if now - created_at > TAB_MAX_AGE_SEC:
                to_close.append(page_id)
        for pid in to_close:
            del page_timestamps[pid]

    for page_id in to_close:
        try:
            for p in list(context.pages):
                if id(p) == page_id:
                    p.close()
                    log(f"closed stale tab (age > {TAB_MAX_AGE_SEC}s)")
                    break
        except Exception as e:
            log(f"cleanup close error: {e}")


def cleanup_loop():
    """Background thread: periodically close old tabs to prevent RAM bloat."""
    while not shutdown_requested:
        time.sleep(CLEANUP_INTERVAL_SEC)
        if shutdown_requested:
            break
        try:
            cleanup_old_tabs()
        except Exception as e:
            log(f"cleanup error: {e}")


def handle_send(body):
    """Process a single send request. Returns (success, result_dict)."""
    global context
    try:
        data = json.loads(body)
    except json.JSONDecodeError as e:
        return False, {"error": f"invalid JSON: {e}"}

    request_id = data.get("request_id")
    message = data.get("message")
    if not request_id or not message:
        return False, {"error": "request_id and message required"}

    image_path = data.get("image_path") or None
    conversation_id = data.get("conversation_id") or None
    base_url = data.get("base_url") or os.environ.get("ELDORADO_BASE_URL", "https://www.eldorado.gg")
    base_url = base_url.rstrip("/")

    if image_path and not os.path.isfile(image_path):
        image_path = None

    if not context:
        return False, {"error": "browser not ready"}

    with semaphore:
        page = None
        try:
            page = context.new_page()
            with page_timestamps_lock:
                page_timestamps[id(page)] = time.time()

            success, route, err = send_chat_message_impl(
                page, request_id, message, image_path, conversation_id, base_url
            )

            if success:
                return True, {"success": True, "request_id": request_id, "route": route}
            return False, {"error": err or "send failed"}
        finally:
            if page:
                try:
                    with page_timestamps_lock:
                        page_timestamps.pop(id(page), None)
                    page.close()
                except Exception as e:
                    log(f"page close error: {e}")


class ChatHandler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        log(f"HTTP {args[0]}")

    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"ok":true}')
        else:
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        if self.path != "/send":
            self.send_response(404)
            self.end_headers()
            return

        content_length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(content_length)

        success, result = handle_send(body)

        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(result).encode())


def run_server():
    server = HTTPServer(("127.0.0.1", PORT), ChatHandler)
    log(f"chat server listening on 127.0.0.1:{PORT}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.shutdown()


def shutdown(signum=None, frame=None):
    global shutdown_requested, browser, playwright
    shutdown_requested = True
    log("shutting down...")
    if browser:
        try:
            browser.close()
        except Exception:
            pass
    if playwright:
        try:
            playwright.stop()
        except Exception:
            pass
    sys.exit(0)


def main():
    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)

    init_browser()

    t = threading.Thread(target=cleanup_loop, daemon=True)
    t.start()

    run_server()


if __name__ == "__main__":
    main()
