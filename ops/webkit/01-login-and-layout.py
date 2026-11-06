#!/usr/bin/env python3
"""Exercise ICS in REAL WebKit (the engine iPad Chrome/Safari actually run)."""
import sys, time
from playwright.sync_api import sync_playwright

import os
BASE = os.environ.get("ICS_TEST_URL", "http://127.0.0.1:8666")
PW = os.environ.get("ICS_TEST_PASSWORD", "66666666")
KEY = os.environ.get("ICS_TEST_KEY", "vip")

results = []
def check(name, cond, detail=""):
    results.append((name, bool(cond), detail))
    print(("  PASS  " if cond else "  FAIL  ") + name + (f"  -> {detail}" if detail and not cond else ""))

def login_with_retry(pg, password, tries=6):
    """Click the login button, waiting out 429s from the edge limit / backoff."""
    for i in range(tries):
        pg.click("#btnLogin")
        pg.wait_for_timeout(1500)
        if pg.evaluate("() => !!localStorage.getItem('ics_token')"):
            return True
        pg.wait_for_timeout(4000)
    return False

def newpage(p, dev="iPad (gen 7)"):
    b = p.webkit.launch(headless=True)
    ctx = b.new_context(**p.devices[dev])
    return b, ctx, ctx.new_page()

def main():
    with sync_playwright() as p:
        print("\n### A. login flow in WebKit")
        b, ctx, pg = newpage(p)
        errs = []
        pg.on("pageerror", lambda e: errs.append(str(e)[:200]))
        posts = []
        pg.on("request", lambda r: posts.append(r.url) if "/api/login" in r.url and r.method == "POST" else None)
        pg.goto(f"{BASE}/?key={KEY}", wait_until="load", timeout=30000)
        pg.wait_for_timeout(600)
        check("no JS errors on load", not errs, errs)
        check("modal visible", pg.locator("#loginModal").is_visible())
        check("password mode", pg.locator("#emailInputContainer").is_visible())
        pg.fill("#passwordField", PW)
        pg.check("#emailInputContainer input[type=checkbox]")
        pg.wait_for_timeout(150)
        check("reveal shows full value", pg.input_value("#passwordField") == PW, pg.input_value("#passwordField"))
        pg.click("#btnLogin")
        pg.wait_for_timeout(1800)
        check("exactly 1 login POST", len(posts) == 1, posts)
        check("logged in", bool(pg.evaluate("()=>localStorage.getItem('ics_token')")))
        check("modal closed", not pg.locator("#loginModal").is_visible())
        check("no JS errors after login", not errs, errs)
        pg.screenshot(path="/tmp/wk-loggedin.png")

        print("\n### B. iOS auto-zoom on input focus (font-size < 16px)")
        fs = pg.evaluate("""() => {
            const el = document.getElementById('textInput');
            return parseFloat(getComputedStyle(el).fontSize);
        }""")
        check("composer font-size >= 16px (no auto-zoom)", fs >= 16, f"{fs}px")

        pwfs = pg.evaluate("() => parseFloat(getComputedStyle(document.getElementById('passwordField')).fontSize)")
        check("password field font-size >= 16px", pwfs >= 16, f"{pwfs}px")

        print("\n### C. touch target sizes (Apple HIG: 44x44pt)")
        small = pg.evaluate("""() => {
            const out = [];
            document.querySelectorAll('button, a, input[type=checkbox]').forEach(el => {
                if (!el.offsetParent) return;
                const r = el.getBoundingClientRect();
                if (r.width === 0 || r.height === 0) return;
                if (r.height < 44 || r.width < 44)
                    out.push({tag: el.tagName, id: el.id || el.className || el.textContent.trim().slice(0,18),
                              w: Math.round(r.width), h: Math.round(r.height)});
            });
            return out;
        }""")
        check("all visible touch targets >= 44px", not small, small[:6])

        print("\n### D. safe-area (notch / home indicator)")
        sa = pg.evaluate("() => document.querySelector('meta[name=viewport]').content")
        check("viewport has viewport-fit=cover", "viewport-fit=cover" in sa, sa)

        print("\n### E. send a message, verify it round-trips")
        pg.fill("#textInput", "webkit smoke test")
        pg.wait_for_timeout(100)
        pg.click("#pushForm button[type=submit]")
        pg.wait_for_timeout(2500)
        body = pg.inner_text("#timeline")
        check("message appears in timeline", "webkit smoke test" in body, body[:120])

        print("\n### F. paste/scroll behaviour + horizontal overflow")
        ow = pg.evaluate("() => document.documentElement.scrollWidth - document.documentElement.clientWidth")
        check("no horizontal overflow", ow <= 1, f"{ow}px overflow")

        print("\n### G. dynamic viewport (iOS toolbar shrink)")
        vh = pg.evaluate("""() => ({
            inner: window.innerHeight,
            vh100: document.documentElement.clientHeight,
            dvhSupported: CSS.supports('height', '100dvh')
        })""")
        print("      ", vh)

        b.close()

        print("\n### H. iPhone SE (smallest modern screen)")
        b2, ctx2, pg2 = newpage(p, "iPhone SE")
        pg2.goto(f"{BASE}/?key={KEY}", wait_until="load", timeout=30000)
        pg2.wait_for_timeout(600)
        ow2 = pg2.evaluate("() => document.documentElement.scrollWidth - document.documentElement.clientWidth")
        check("iPhone SE: no horizontal overflow", ow2 <= 1, f"{ow2}px")
        modal_fits = pg2.evaluate("""() => {
            const m = document.querySelector('#loginModal > div');
            const r = m.getBoundingClientRect();
            return {top: Math.round(r.top), bottom: Math.round(r.bottom), vh: window.innerHeight};
        }""")
        check("iPhone SE: login card fits vertically",
              modal_fits["top"] >= 0 and modal_fits["bottom"] <= modal_fits["vh"], modal_fits)
        pg2.screenshot(path="/tmp/wk-iphonese.png")
        b2.close()

    bad = [r for r in results if not r[1]]
    print(f"\n{'ALL PASS' if not bad else 'FAILURES: ' + str([b[0] for b in bad])}")
    return 1 if bad else 0

sys.exit(main())
