#!/usr/bin/env python3
"""
Eldorado browser login helper using patchright (patched Playwright).
Uses real Chrome (not Chromium) and handles Cloudflare Turnstile automatically.
"""

import base64
import json
import os
import sys
import time

from patchright.sync_api import sync_playwright


def main():
    if len(sys.argv) < 4:
        print(json.dumps({"error": "Usage: browser_login.py <base_url> <email> <password>"}))
        sys.exit(1)

    base_url = sys.argv[1]
    email = sys.argv[2]
    password = sys.argv[3]

    log("starting patchright login for " + email)

    with sync_playwright() as p:
        browser = p.chromium.launch(
            channel="chrome",
            headless=False,
            args=[
                "--disable-blink-features=AutomationControlled",
                "--no-sandbox",
            ],
        )
        context = browser.new_context(no_viewport=True)
        page = context.new_page()

        # Step 1: Navigate to homepage
        log("navigating to " + base_url)
        page.goto(base_url, wait_until="domcontentloaded", timeout=30000)
        wait_for_cloudflare(page, "homepage")

        # Step 2: Click login button
        log("clicking login button")
        try:
            page.click('button[aria-label="Log in"]', timeout=10000)
        except Exception:
            try:
                page.click('button.button__primary', timeout=5000)
            except Exception:
                page.locator("text=Log in").first.click(timeout=5000)

        page.wait_for_timeout(3000)
        log("current URL: " + page.url[:120])

        # Step 3: Handle Cloudflare Turnstile on login.eldorado.gg
        wait_for_cloudflare(page, "login page", timeout=90)

        # Step 4: Wait for login form
        log("waiting for login form")
        form_found = wait_for_form(page)
        if not form_found:
            log("ERROR: login form not found")
            print(json.dumps({"error": "login form not found after 60s"}))
            sys.exit(1)

        # Step 5: Fill email only (Cognito two-step: email first, then password)
        log("filling email")
        fill_email(page, email)
        page.wait_for_timeout(500)

        # Step 6: Submit email form
        log("submitting email")
        submit_form(page)
        page.wait_for_timeout(3000)

        # Step 7: Wait for password page (verifyPassword)
        log("waiting for password page")
        wait_for_password_page(page)

        # Step 8: Fill password and submit
        log("filling password")
        fill_password(page, password)
        page.wait_for_timeout(500)
        log("submitting password")
        submit_form(page)
        page.wait_for_timeout(5000)

        # Step 9: Wait for auth cookies
        log("waiting for auth cookies")
        cookies = wait_for_auth_cookies(context, page)
        if cookies is None:
            log("ERROR: auth cookies not received")
            print(json.dumps({"error": "auth cookies not received after 60s"}))
            sys.exit(1)

        # Step 10: Intercept a real API request to capture the exact Cookie header the browser sends
        log("intercepting browser request to capture raw Cookie header...")
        captured = {"cookie_header": "", "xsrf_header": ""}

        def capture_request(route, request):
            hdrs = request.headers
            captured["cookie_header"] = hdrs.get("cookie", "")
            captured["xsrf_header"] = hdrs.get("x-xsrf-token", "")
            route.continue_()

        page.route("**/api/boostingOffers/me/boostingSubscriptions", capture_request)

        verify_result = page.evaluate("""
            () => fetch('/api/boostingOffers/me/boostingSubscriptions', {
                credentials: 'include',
                headers: { 'Accept': 'application/json' }
            }).then(r => ({ status: r.status, ok: r.ok }))
              .catch(e => ({ status: 0, error: e.message }))
        """)
        log(f"in-browser API test: {verify_result}")

        page.unroute("**/api/boostingOffers/me/boostingSubscriptions")

        raw_cookie = captured["cookie_header"]
        raw_xsrf_header = captured["xsrf_header"]
        log(f"captured Cookie header length: {len(raw_cookie)}")
        log(f"captured X-XSRF-TOKEN header: {raw_xsrf_header[:40] if raw_xsrf_header else 'NONE'}...")

        # Extract XSRF token from the raw Cookie header sent by the browser
        xsrf_from_cookie = ""
        for part in raw_cookie.split("; "):
            if part.startswith("__Host-XSRF-TOKEN="):
                xsrf_from_cookie = part[len("__Host-XSRF-TOKEN="):]
                log(f"__Host-XSRF-TOKEN from raw Cookie header: {xsrf_from_cookie[:40]}...")
            elif part.startswith("XSRF-TOKEN="):
                xsrf_val = part[len("XSRF-TOKEN="):]
                log(f"XSRF-TOKEN from raw Cookie header: {xsrf_val[:40]}...")

        # Step 11: Fetch TalkJS token (from messages/chat authorize API)
        talkjs_token = ""
        try:
            log("fetching TalkJS token from Eldorado authorize...")
            auth_result = page.evaluate("""
                () => fetch('/api/conversations/me/authorize', {
                    credentials: 'include',
                    headers: { 'Accept': 'application/json' }
                }).then(r => r.json()).then(data => ({
                    ok: true,
                    token: data.token || data.jwt || data.Jwt || data.accessToken || data.talkJsToken
                        || (data.data && (data.data.token || data.data.jwt))
                        || (typeof data === 'string' ? data : null),
                    raw: JSON.stringify(data).slice(0, 200)
                })).catch(e => ({ ok: false, error: e.message }))
            """)
            if auth_result.get("ok") and auth_result.get("token"):
                talkjs_token = auth_result["token"].strip()
                log(f"TalkJS token fetched, len={len(talkjs_token)}")
            else:
                # Fallback: try alternate endpoints
                for path in ["/api/users/me/talkJsToken", "/api/chat/talkJsToken"]:
                    try:
                        alt = page.evaluate(f"""
                            () => fetch('{path}', {{ credentials: 'include', headers: {{ 'Accept': 'application/json' }} }})
                                .then(r => r.json()).then(d => d.token || d.jwt || d.accessToken || null)
                                .catch(() => null)
                        """)
                        if alt and len(str(alt)) > 50:
                            talkjs_token = str(alt).strip()
                            log(f"TalkJS token from {path}, len={len(talkjs_token)}")
                            break
                    except Exception:
                        pass
            if not talkjs_token:
                log(f"TalkJS token not found (auth_result={auth_result})")
        except Exception as e:
            log(f"TalkJS token fetch failed: {e}")

        # Use the raw Cookie header from the browser - this is exactly what works
        result = {
            "cookies": raw_cookie,
            "xsrf_token": xsrf_from_cookie or raw_xsrf_header,
        }
        if talkjs_token:
            result["talkjs_token"] = talkjs_token
        log(f"login successful! cookie length={len(result['cookies'])}")

        # Save structured cookies for reuse by other scripts (e.g. send_chat_message.py)
        storage_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "storage")
        os.makedirs(storage_dir, exist_ok=True)
        cookies_file = os.path.join(storage_dir, "browser_cookies.json")
        with open(cookies_file, "w") as f:
            json.dump(cookies, f)
        log(f"saved {len(cookies)} cookies to {cookies_file}")

        # Save TalkJS token for curl-based message send (auto-rotate on login)
        if talkjs_token:
            try:
                payload_b64 = talkjs_token.split(".")[1]
                payload_b64 += "=" * (4 - len(payload_b64) % 4) if len(payload_b64) % 4 else ""
                payload = json.loads(base64.urlsafe_b64decode(payload_b64).decode())
                exp = payload.get("exp", 0)
            except Exception:
                exp = int(time.time()) + 86400  # default 24h
            token_file = os.path.join(storage_dir, "talkjs_token.json")
            with open(token_file, "w") as f:
                json.dump({"token": talkjs_token, "expires_at": exp}, f)
            log(f"saved TalkJS token to {token_file} (exp={exp})")

        browser.close()
        print(json.dumps(result))


def wait_for_cloudflare(page, page_name, timeout=90):
    """Wait for Cloudflare challenge, clicking Turnstile checkbox if present."""
    deadline = time.time() + timeout
    turnstile_clicked = False

    while time.time() < deadline:
        try:
            title = page.title().lower()
        except Exception:
            # Navigation in progress (Cloudflare resolving), wait and retry
            page.wait_for_timeout(3000)
            try:
                page.wait_for_load_state("domcontentloaded", timeout=10000)
            except Exception:
                pass
            continue

        is_challenge = any(kw in title for kw in [
            "just a moment", "bir dakika", "attention required", "cloudflare",
        ])

        if not is_challenge:
            log(f"Cloudflare passed on {page_name} (title: {page.title()[:60]})")
            return

        # Try to find and click the Turnstile checkbox in the iframe
        if not turnstile_clicked:
            turnstile_clicked = try_click_turnstile(page)
            if turnstile_clicked:
                # After clicking, wait for the page to navigate
                page.wait_for_timeout(5000)
                continue

        remaining = int(deadline - time.time())
        log(f"Cloudflare challenge on {page_name}... remaining: {remaining}s")
        page.wait_for_timeout(2000)

    raise Exception(f"Cloudflare challenge did not resolve on {page_name} within {timeout}s")


def try_click_turnstile(page):
    """Try to find and click the Cloudflare Turnstile checkbox."""
    try:
        # Look for Turnstile iframe
        for frame in page.frames:
            if "challenges.cloudflare.com" in frame.url or "turnstile" in frame.url.lower():
                log(f"found Turnstile iframe: {frame.url[:80]}")
                try:
                    # Try clicking the checkbox inside the iframe
                    checkbox = frame.query_selector('input[type="checkbox"]')
                    if checkbox:
                        checkbox.click()
                        log("clicked Turnstile checkbox (input)")
                        return True

                    # Try clicking the label/div that wraps the checkbox
                    body = frame.query_selector("body")
                    if body:
                        body.click()
                        log("clicked Turnstile body")
                        return True
                except Exception as e:
                    log(f"Turnstile click failed: {e}")

        # Try clicking the Turnstile widget directly on the main page
        turnstile_sel = [
            'iframe[src*="challenges.cloudflare.com"]',
            'iframe[src*="turnstile"]',
            '.cf-turnstile',
            '#turnstile-wrapper',
            '[data-sitekey]',
        ]
        for sel in turnstile_sel:
            el = page.query_selector(sel)
            if el:
                log(f"found Turnstile element: {sel}")
                box = el.bounding_box()
                if box:
                    # Click in the center of the element
                    page.mouse.click(box["x"] + box["width"] / 2, box["y"] + box["height"] / 2)
                    log("clicked Turnstile element center")
                    return True
    except Exception as e:
        log(f"Turnstile detection error: {e}")

    return False


def wait_for_form(page):
    """Wait for the Cognito login form to appear."""
    form_selectors = [
        'input[name="username"]',
        'input[id="signInFormUsername"]',
        'input[name="email"]',
        'input[type="email"]',
    ]
    for attempt in range(30):
        for sel in form_selectors:
            el = page.query_selector(sel)
            if el:
                log(f"found login form via: {sel}")
                return True
        log(f"waiting for login form... ({attempt+1}/30)")
        page.wait_for_timeout(2000)
    return False


def fill_email(page, email):
    """Fill email on the first Cognito page."""
    selectors = [
        'input[name="username"]', 'input[id="signInFormUsername"]',
        'input[name="email"]', 'input[type="email"]',
    ]
    for sel in selectors:
        el = page.query_selector(sel)
        if el:
            el.fill(email)
            log(f"email filled via: {sel}")
            return
    raise Exception("could not find email input")


def wait_for_password_page(page):
    """Wait for the verifyPassword page to load."""
    for attempt in range(20):
        url = page.url.lower()
        if "verifypassword" in url or "verify-password" in url:
            log("password page loaded")
            page.wait_for_timeout(1000)
            return

        pw_el = page.query_selector('input[type="password"]')
        if pw_el:
            try:
                body = page.inner_text("body")[:200]
                if "password" in body.lower():
                    log("password page detected via content")
                    return
            except Exception:
                pass

        log(f"waiting for password page... ({attempt+1}/20)")
        page.wait_for_timeout(1500)
    raise Exception("password page did not load within 30s")


def fill_password(page, password):
    """Fill password on the second Cognito page."""
    selectors = [
        'input[name="password"]', 'input[id="signInFormPassword"]',
        'input[type="password"]',
    ]
    for sel in selectors:
        el = page.query_selector(sel)
        if el:
            el.fill(password)
            log(f"password filled via: {sel}")
            return
    raise Exception("could not find password input")


def submit_form(page):
    """Submit the login form."""
    submit_selectors = [
        'input[name="signInSubmitButton"]',
        'button[name="signInSubmitButton"]',
        'button[type="submit"]',
        'input[type="submit"]',
    ]
    for sel in submit_selectors:
        el = page.query_selector(sel)
        if el:
            el.click()
            return
    page.keyboard.press("Enter")


def wait_for_auth_cookies(context, page):
    """Wait for __Host-EldoradoIdToken cookie to appear."""
    for attempt in range(40):
        cookies = context.cookies()
        for c in cookies:
            if c["name"] == "__Host-EldoradoIdToken" and c["value"]:
                return cookies

        current_url = page.url[:100]

        # Check for Cloudflare challenge on current page
        try:
            title = page.title().lower()
            if any(kw in title for kw in ["just a moment", "bir dakika", "cloudflare"]):
                log(f"Cloudflare challenge detected at {current_url}, trying to solve...")
                try_click_turnstile(page)
                page.wait_for_timeout(5000)
                continue
        except Exception:
            pass

        # Debug: check for error messages on verifyPassword page
        if attempt == 5 and "verifyPassword" in current_url:
            try:
                body_text = page.inner_text("body")[:500]
                log(f"verifyPassword page content: {body_text}")
            except Exception:
                pass

        # Check if we redirected back to eldorado
        if "www.eldorado.gg" in current_url and "auth-callback" not in current_url:
            log(f"redirected back to Eldorado: {current_url}")
            page.wait_for_timeout(3000)
            cookies = context.cookies()
            for c in cookies:
                if c["name"] == "__Host-EldoradoIdToken" and c["value"]:
                    return cookies

        log(f"waiting for auth cookies... URL: {current_url} ({attempt+1}/40)")
        page.wait_for_timeout(2000)
    return None


def build_cookie_result(cookies, browser_xsrf=None):
    """Build cookie string and extract XSRF token."""
    important = {
        "cf_clearance", "__Host-EldoradoIdToken", "__Host-EldoradoRefreshToken",
        "__Host-XSRF-TOKEN", "XSRF-TOKEN", "pseudoId", "x-session-id",
        "eldoradogg_locale", "eldoradogg_currencyPreference",
    }

    parts = []

    for c in cookies:
        if c["name"] in important:
            parts.append(f'{c["name"]}={c["value"]}')

    for c in cookies:
        if c["name"] not in important and "eldorado" in c.get("domain", ""):
            parts.append(f'{c["name"]}={c["value"]}')

    # Use the XSRF token as read by JavaScript (document.cookie) - this is
    # exactly what Angular's HttpXsrfInterceptor reads and sends as X-XSRF-TOKEN header.
    # If XSRF-TOKEN is HttpOnly (not visible to JS), fall back to __Host-XSRF-TOKEN from JS.
    xsrf_token = ""
    if browser_xsrf:
        xsrf_token = browser_xsrf.get("xsrf") or browser_xsrf.get("hostXsrf") or ""

    # Fallback to context.cookies() values if document.cookie didn't have it
    if not xsrf_token:
        for c in cookies:
            if c["name"] == "XSRF-TOKEN":
                xsrf_token = c["value"]
                break
        if not xsrf_token:
            for c in cookies:
                if c["name"] == "__Host-XSRF-TOKEN":
                    xsrf_token = c["value"]
                    break

    log(f"final XSRF token source: len={len(xsrf_token)} value={xsrf_token[:50]}...")

    return {"cookies": "; ".join(parts), "xsrf_token": xsrf_token}


def log(msg):
    print(f"[patchright] {msg}", file=sys.stderr)


if __name__ == "__main__":
    main()
