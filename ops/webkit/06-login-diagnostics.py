#!/usr/bin/env python3
"""Login failure diagnostics: evidence panel, input normalisation, bad replies.

Pins the behaviours added after a long 'it just errors' hunt:
  - a failed login shows exactly what the browser sent (URL, key, code points,
    byte count, HTTP status, raw reply) so no one has to guess
  - full-width digits from a CJK IME are normalised before submit
  - a non-JSON reply (proxy / captive portal / Cloudflare) yields a readable
    message, not a raw SyntaxError
"""
import os
import sys
from playwright.sync_api import sync_playwright

BASE = os.environ.get("ICS_TEST_URL", "http://127.0.0.1:8666")
PW = os.environ.get("ICS_TEST_PASSWORD", "66666666")
KEY = os.environ.get("ICS_TEST_KEY", "vip")

res = []
def ck(name, cond, detail=""):
    res.append((name, bool(cond), detail))
    print(("  PASS  " if cond else "  FAIL  ") + name + (f"  -> {detail}" if detail and not cond else ""))

def launch(p):
    return p.webkit.launch(headless=True)

def open_login(pg, url):
    pg.goto(url, wait_until="load", timeout=30000)
    pg.wait_for_timeout(700)
    if not pg.locator("#emailInputContainer").is_visible():
        pg.click("#toggleLoginMode")
        pg.wait_for_timeout(300)

with sync_playwright() as p:
    b = launch(p)

    print("### A. evidence panel on a wrong key")
    ctx = b.new_context(**p.devices["iPad (gen 7)"]); pg = ctx.new_page()
    open_login(pg, f"{BASE}/?key=definitely-wrong")
    pg.fill("#passwordField", PW)
    pg.click("#btnLogin"); pg.wait_for_timeout(2000)

    ev = pg.locator("#loginEvidence")
    ck("evidence panel is visible", ev.is_visible())
    txt = ev.inner_text() if ev.is_visible() else ""
    ck("shows the page URL as it arrived", "key=definitely-wrong" in txt, txt[:120])
    ck("shows the key that was submitted", '"definitely-wrong"' in txt)
    ck("shows password char and byte counts", "字符数" in txt and "字节数" in txt)
    ck("shows the code points", "码点" in txt)
    ck("shows the HTTP status", "401" in txt)
    ck("shows the raw server reply", "服务器原文" in txt)
    ck("no raw SyntaxError leaked", "Unexpected token" not in txt)
    ctx.close()

    print("\n### B. full-width digits are normalised before submit")
    ctx = b.new_context(**p.devices["iPad (gen 7)"]); pg = ctx.new_page()
    open_login(pg, f"{BASE}/?key={KEY}")
    pg.fill("#passwordField", "")
    pg.type("#passwordField", "６６６６６６６６")   # full-width form of the local password
    pg.wait_for_timeout(400)
    ck("full-width converted to ASCII", pg.input_value("#passwordField") == "66666666",
       pg.input_value("#passwordField"))
    pg.click("#btnLogin"); pg.wait_for_timeout(2200)
    ck("login succeeds after normalisation",
       bool(pg.evaluate("()=>localStorage.getItem('ics_token')")))
    ctx.close()

    print("\n### C. non-JSON reply is readable, not a SyntaxError")
    ctx = b.new_context(**p.devices["iPad (gen 7)"]); pg = ctx.new_page()
    open_login(pg, f"{BASE}/?key={KEY}")
    pg.route("**/api/login", lambda r: r.fulfill(
        status=200, content_type="text/html",
        body="<!DOCTYPE html><html><body>WiFi login portal</body></html>"))
    pg.fill("#passwordField", PW)
    pg.click("#btnLogin"); pg.wait_for_timeout(1600)
    err = pg.locator("#loginError").inner_text() if pg.locator("#loginError").is_visible() else ""
    ck("readable message about proxies/portals",
       ("proxy" in err.lower() or "portal" in err.lower()) and "Unexpected token" not in err, err)
    pg.unroute("**/api/login")
    ctx.close()

    print("\n### D. 429 from the edge shows the status code")
    ctx = b.new_context(**p.devices["iPad (gen 7)"]); pg = ctx.new_page()
    open_login(pg, f"{BASE}/?key={KEY}")
    pg.route("**/api/login", lambda r: r.fulfill(
        status=429, content_type="text/html", body="<html>Rate limited</html>"))
    pg.fill("#passwordField", PW)
    pg.click("#btnLogin"); pg.wait_for_timeout(1600)
    err = pg.locator("#loginError").inner_text() if pg.locator("#loginError").is_visible() else ""
    ck("429 surfaces the status", "429" in err, err)
    ck("no SyntaxError", "Unexpected token" not in err, err)
    pg.unroute("**/api/login")
    ctx.close()

    b.close()

bad = [r for r in res if not r[1]]
print(f"\n{'ALL PASS' if not bad else 'FAILURES: ' + str([x[0] for x in bad])}")
sys.exit(1 if bad else 0)
