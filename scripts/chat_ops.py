"""
Shared chat operations for Eldorado TalkJS - used by send_chat_message.py and chat_server.py.
"""

import json
import os
import sys
import tempfile
import time

# Görsel optimizasyonu: max boyut (bytes)
IMAGE_MAX_BYTES = int(os.environ.get("CHAT_IMAGE_MAX_KB", "200")) * 1024

COOKIES_FILE = os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "..", "storage", "browser_cookies.json"
)


def log(msg):
    print(f"[chat-ops] {msg}", file=sys.stderr)


def optimize_image(image_path):
    """
    Görseli sıkıştır (max ~200KB). Pillow yoksa orijinali döndür.
    """
    if not os.path.isfile(image_path):
        return image_path
    if os.path.getsize(image_path) <= IMAGE_MAX_BYTES:
        return image_path
    try:
        from PIL import Image

        with Image.open(image_path) as img:
            if img.mode in ("RGBA", "P"):
                img = img.convert("RGB")
            w, h = img.size
            if w > 1200 or h > 1200:
                resample = getattr(Image.Resampling, "LANCZOS", Image.LANCZOS)
                img.thumbnail((1200, 1200), resample)
            out_path = tempfile.NamedTemporaryFile(suffix=".jpg", delete=False).name
            quality = 85
            while quality >= 50:
                img.save(out_path, "JPEG", quality=quality, optimize=True)
                if os.path.getsize(out_path) <= IMAGE_MAX_BYTES:
                    log(f"image optimized: {os.path.getsize(image_path)//1024}KB -> {os.path.getsize(out_path)//1024}KB")
                    return out_path
                quality -= 10
            return out_path
    except ImportError:
        return image_path
    except Exception as e:
        log(f"image optimize failed: {e}, using original")
        return image_path


def load_cookies():
    """Load and return valid cookie dicts for Playwright context.add_cookies()."""
    if not os.path.isfile(COOKIES_FILE):
        return None
    with open(COOKIES_FILE) as f:
        cookies = json.load(f)
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
    return valid_cookies


def find_talkjs_frame(page):
    """Find the TalkJS chat iframe."""
    for attempt in range(10):
        for frame in page.frames:
            frame_url = frame.url.lower()
            if "talkjs" in frame_url or "chatbox" in frame_url:
                log(f"found TalkJS frame: {frame.url[:80]}")
                return frame
        log(f"waiting for TalkJS frame... ({attempt + 1}/10)")
        page.wait_for_timeout(500)

    log("TalkJS frame not found after 10 attempts")
    return None


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

        page.wait_for_timeout(1500)


def open_chat_with_direct_first(page, base_url, request_id, conversation_id):
    """Go to boosting-request page, click Chat with buyer. Most reliable path."""
    url = f"{base_url}/boosting-request/{request_id}"
    log(f"navigating to {url}")
    page.goto(url, wait_until="domcontentloaded", timeout=45000)
    wait_for_cloudflare(page, "request-detail")
    page.wait_for_timeout(2500)
    log(f"page title: {page.title()}")
    log(f"current URL: {page.url[:100]}")

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
        page.wait_for_timeout(800)

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
        return None, "fallback"

    chat_btn.click()
    log("clicked 'Chat with buyer'")
    page.wait_for_timeout(2500)
    return find_talkjs_frame(page), "fallback"


def close_upload_overlay(frame, page):
    """Try to close TalkJS upload preview overlay so input is clickable again."""
    for _ in range(2):
        try:
            page.keyboard.press("Escape")
            page.wait_for_timeout(250)
        except Exception:
            pass

    for sel in [
        'button[aria-label*="Close"]',
        'button[title*="Close"]',
        'button:has-text("Cancel")',
        '[data-testid*="close"]',
        '[class*="close"]',
    ]:
        try:
            btn = frame.query_selector(sel)
            if btn and btn.is_visible():
                btn.click(timeout=1000)
                page.wait_for_timeout(250)
                log(f"closed upload overlay: {sel}")
                return
        except Exception:
            continue


def was_message_likely_sent(chat_input, original_text):
    """Best-effort send verification: if input still contains full text, treat as unsent."""
    try:
        current = chat_input.evaluate(
            """
            (el) => {
                const tag = el.tagName ? el.tagName.toLowerCase() : '';
                if (tag === 'textarea' || tag === 'input') return (el.value || '').trim();
                if (el.isContentEditable) return (el.textContent || '').trim();
                return (el.innerText || '').trim();
            }
            """
        )
    except Exception:
        return True

    src = (original_text or "").strip()
    cur = (current or "").strip()
    if not src:
        return True
    return not (cur and (cur == src or src.startswith(cur) or cur.startswith(src[: max(1, len(src) - 5)])))


def send_message(frame, text, page):
    """Set message via JavaScript (React-compatible) and send."""
    close_upload_overlay(frame, page)

    chat_input = None
    for selector in [
        '[placeholder="Say something..."]',
        'textarea',
        '[role="textbox"]',
        'div[contenteditable="true"]',
        '[class*="message-input"]',
        '[class*="MessageInput"]',
    ]:
        try:
            el = frame.query_selector(selector)
            if el and el.is_visible():
                chat_input = el
                log(f"found chat input: {selector}")
                break
        except Exception:
            continue

    if not chat_input:
        log("ERROR: chat input not found in TalkJS frame")
        return False

    try:
        chat_input.evaluate("(el) => el.focus()")
    except Exception:
        pass
    try:
        chat_input.click(timeout=800)
    except Exception:
        pass
    frame.wait_for_timeout(250)

    def set_value_and_emit(element, value):
        try:
            return element.evaluate(
                """
                (el, value) => {
                    el.focus();
                    const tag = el.tagName ? el.tagName.toLowerCase() : '';
                    if (tag === 'textarea' || tag === 'input') {
                        const Proto = tag === 'textarea' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
                        const proto = Object.getOwnPropertyDescriptor(Proto, 'value');
                        if (proto && proto.set) {
                            proto.set.call(el, value);
                        } else {
                            el.value = value;
                        }
                        el.dispatchEvent(new InputEvent('input', { bubbles: true, data: value }));
                        el.dispatchEvent(new Event('change', { bubbles: true }));
                    } else if (el.isContentEditable) {
                        const escaped = (value || '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
                        el.innerHTML = escaped.replace(/\\n/g, '<br>');
                        el.dispatchEvent(new InputEvent('input', { bubbles: true }));
                    }
                    return true;
                }
                """,
                value,
            )
        except Exception as e:
            log(f"evaluate failed: {e}")
            return False

    ok = set_value_and_emit(chat_input, text)
    if not ok:
        try:
            chat_input.fill(text)
        except Exception as e:
            log(f"fill failed: {e}")
            return False

    frame.wait_for_timeout(350)

    for _ in range(4):
        for send_selector in [
            'button:has-text("Send")',
            'button[type="submit"]',
            '[aria-label="Send"]',
            '[data-testid="send-button"]',
        ]:
            try:
                btn = frame.query_selector(send_selector)
                if btn and btn.is_visible():
                    try:
                        btn.click(timeout=1000)
                    except Exception:
                        btn.click(force=True, timeout=1000)
                    log(f"clicked Send: {send_selector}")
                    frame.wait_for_timeout(350)
                    if was_message_likely_sent(chat_input, text):
                        return True
            except Exception:
                continue
        frame.wait_for_timeout(200)

    try:
        chat_input.evaluate(
            """
            (el) => {
                const ev = new KeyboardEvent('keydown', { key: 'Enter', code: 'Enter', keyCode: 13, which: 13, bubbles: true });
                el.dispatchEvent(ev);
            }
            """
        )
        frame.wait_for_timeout(350)
        chat_input.press("Enter")
    except Exception as e:
        log(f"Enter fallback: {e}")
    frame.wait_for_timeout(450)
    return was_message_likely_sent(chat_input, text)


def click_image_send(frame, page):
    """Try multiple strategies to click TalkJS upload preview send action."""
    selectors = [
        'button:has-text("Send")',
        'button[type="submit"]',
        '[data-testid="send-button"]',
        '[aria-label="Send"]',
        '.send-row button',
        '[class*="send-row"] button',
    ]

    for sel in selectors:
        for scope_name, scope in [("iframe", frame), ("page", page)]:
            try:
                btn = scope.query_selector(sel)
                if btn and btn.is_visible():
                    try:
                        btn.click(timeout=3000)
                    except Exception:
                        btn.click(force=True, timeout=3000)
                    log(f"clicked image Send ({scope_name}): {sel}")
                    return True
            except Exception:
                continue

    for scope_name, scope in [("iframe", frame), ("page", page)]:
        try:
            clicked = scope.evaluate(
                """
                () => {
                    const roots = Array.from(document.querySelectorAll('.AttachOverlay, [class*="UploadPreview"], [class*="attach"]'));
                    const candidates = [];
                    const pushIfVisible = (el) => {
                        if (!el) return;
                        const r = el.getBoundingClientRect();
                        if (r.width > 0 && r.height > 0) candidates.push(el);
                    };
                    for (const root of roots) {
                        root.querySelectorAll('button, a, [role="button"]').forEach(pushIfVisible);
                    }
                    for (const el of candidates) {
                        const txt = (el.textContent || '').trim().toLowerCase();
                        const aria = (el.getAttribute('aria-label') || '').toLowerCase();
                        if (txt.includes('send') || aria.includes('send')) {
                            el.click();
                            return true;
                        }
                    }
                    return false;
                }
                """
            )
            if clicked:
                log(f"clicked image Send (JS {scope_name})")
                return True
        except Exception:
            continue

    try:
        frame.keyboard.press("Enter")
        page.wait_for_timeout(300)
        return True
    except Exception:
        pass
    try:
        page.keyboard.press("Enter")
        page.wait_for_timeout(300)
        return True
    except Exception:
        return False


def send_image(frame, image_path, page):
    """Upload an image to the TalkJS chat and click Send."""
    abs_path = os.path.abspath(image_path)
    log(f"uploading image: {abs_path}")

    for attempt in range(5):
        try:
            file_input = frame.query_selector('input[type="file"]')
            if file_input:
                file_input.set_input_files(abs_path)
                page.wait_for_timeout(3200)
                log("image selected via file input")

                send_clicked = click_image_send(frame, page)
                page.wait_for_timeout(1800)
                if send_clicked:
                    log("image sent successfully")
                    return True

                log("WARNING: image send action not confirmed, retrying...")
                close_upload_overlay(frame, page)
        except Exception as e:
            log(f"image upload attempt {attempt + 1}: {e}")
        page.wait_for_timeout(1500)

    log("WARNING: could not upload image after retries")
    return False


def send_chat_message_impl(page, request_id, message_text, image_path, conversation_id, base_url):
    """
    Core implementation: send message using an existing page.
    Returns (success: bool, route: str, error: str or None).
    """
    talkjs_frame, route = open_chat_with_direct_first(page, base_url, request_id, conversation_id)
    if not talkjs_frame:
        return False, "fallback", "TalkJS chat iframe not found"

    if image_path and os.path.isfile(image_path):
        optimized = optimize_image(image_path)
        try:
            send_image(talkjs_frame, optimized, page)
        finally:
            if optimized != image_path and os.path.isfile(optimized):
                try:
                    os.remove(optimized)
                except Exception:
                    pass
        close_upload_overlay(talkjs_frame, page)
        page.wait_for_timeout(1000)

    success = send_message(talkjs_frame, message_text, page)
    if not success:
        return False, route, "could not send message - input not found"

    page.wait_for_timeout(700)
    log("message sent successfully")
    return True, route, None
