#!/usr/bin/env python3
"""Regression test for ICS login UI — iPad WebKit-ish emulation.

Checks the specific defects fixed in V12:
  1. Login button fires exactly ONE request (no ontouchstart double-fire).
  2. Login title/subtitle follow the active mode (i18n no longer clobbers them).
  3. Toggle button label matches the active mode.
  4. Show-password reveals the FULL value, not just the last char.
  5. Trailing slash in ?key= is tolerated.
  6. Diagnostic panel hidden unless ?debug=1.
"""
import sys, time
from playwright.sync_api import sync_playwright

import os
BASE = os.environ.get("ICS_TEST_URL", "http://127.0.0.1:8666")
PASSWORD = os.environ.get("ICS_TEST_PASSWORD", "66666666")
KEY = os.environ.get("ICS_TEST_KEY", "vip")

failures = []
def check(name, cond, detail=""):
    print(("  PASS  " if cond else "  FAIL  ") + name + (("  -> " + str(detail)) if detail and not cond else ""))
    if not cond:
        failures.append(name)

def run(p, url, label):
    print(f"\n=== {label} === {url}")
    browser = p.chromium.launch(headless=True)
    ctx = browser.new_context(**p.devices["iPad (gen 7)"])
    page = ctx.new_page()

    login_posts = []
    page.on("request", lambda r: login_posts.append(r.url) if "/api/login" in r.url and r.method == "POST" else None)

    page.goto(url, wait_until="networkidle", timeout=30000)
    page.wait_for_timeout(400)

    check("login modal shown", page.locator("#loginModal").is_visible())
    check("password mode active", page.locator("#emailInputContainer").is_visible())

    # 2. title/subtitle follow the mode
    title = page.locator("#loginTitle").inner_text()
    check("title is password-mode title", "Password" in title or "密码" in title, title)

    # 3. toggle label offers the *other* mode
    toggle = page.locator("#toggleLoginMode").inner_text()
    check("toggle offers token mode", "token" in toggle.lower() or "Token" in toggle, toggle)

    # 6. debug panel hidden by default
    debug_visible = page.locator("#debugPanel").is_visible()
    check("debug panel hidden without ?debug=1", not debug_visible, debug_visible)

    # 4. show-password reveals the whole value
    page.fill("#passwordField", PASSWORD)
    page.check("#emailInputContainer input[type=checkbox]")
    page.wait_for_timeout(150)
    revealed = page.input_value("#passwordField")
    input_type = page.get_attribute("#passwordField", "type")
    check("show-password switches to type=text", input_type == "text", input_type)
    check("show-password keeps full value", revealed == PASSWORD, repr(revealed))

    # 1. exactly one login request per click
    page.click("#btnLogin")
    page.wait_for_timeout(1500)
    check("exactly one /api/login POST per click", len(login_posts) == 1, login_posts)

    # 5. logged in
    logged_in = page.evaluate("() => !!localStorage.getItem('ics_token')")
    check("token stored after login", logged_in)

    page.wait_for_timeout(800)
    check("timeline visible", page.locator("#timeline").is_visible())

    # channel dropdown populated (needs auth token)
    opts = page.evaluate("() => Array.from(document.querySelectorAll('#channelSelect option')).map(o=>o.value)")
    check("channel dropdown has the default option", "" in opts, opts)

    page.screenshot(path=f"/tmp/ics-v12-{label}.png", full_page=False)
    browser.close()

def main():
    with sync_playwright() as p:
        run(p, f"{BASE}/?key={KEY}", "clean-key")
        run(p, f"{BASE}/?key={KEY}/", "trailing-slash")
    print("\n" + ("ALL PASS" if not failures else f"FAILURES: {failures}"))
    sys.exit(1 if failures else 0)

main()
