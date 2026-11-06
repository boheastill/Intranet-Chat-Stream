#!/usr/bin/env python3
"""Deeper WebKit/iOS behaviour probes."""
import sys
from playwright.sync_api import sync_playwright

import os
BASE=os.environ.get("ICS_TEST_URL","http://127.0.0.1:8666")
PW=os.environ.get("ICS_TEST_PASSWORD","66666666")
KEY=os.environ.get("ICS_TEST_KEY","vip")
res=[]
def ck(n,c,d=""):
    res.append((n,bool(c),d)); print(("  PASS  " if c else "  FAIL  ")+n+(f"  -> {d}" if d and not c else ""))

def login_with_retry(pg, password, tries=6):
    """Click the login button, waiting out 429s from the edge limit / backoff."""
    for i in range(tries):
        pg.click("#btnLogin")
        pg.wait_for_timeout(1500)
        if pg.evaluate("() => !!localStorage.getItem('ics_token')"):
            return True
        pg.wait_for_timeout(4000)
    return False

def login(pg):
    pg.goto(f"{BASE}/?key={KEY}", wait_until="load", timeout=30000)
    pg.wait_for_timeout(500)
    pg.fill("#passwordField", PW)
    login_with_retry(pg, PW)

with sync_playwright() as p:
    b=p.webkit.launch(headless=True)
    ctx=b.new_context(**p.devices["iPad (gen 7)"])
    pg=ctx.new_page()
    errs=[]; pg.on("pageerror", lambda e: errs.append(str(e)[:200]))
    login(pg)

    print("\n### I. double-tap zoom on buttons (touch-action)")
    ta = pg.evaluate("""() => {
        const r = {};
        ['#btnLogin','#langToggle','#btnLogoutHeader'].forEach(s=>{
            const el=document.querySelector(s); if(el) r[s]=getComputedStyle(el).touchAction;
        });
        const sub=document.querySelector('#pushForm button[type=submit]');
        if(sub) r['send']=getComputedStyle(sub).touchAction;
        return r;
    }""")
    ck("interactive controls use touch-action:manipulation",
       all(v in ("manipulation","auto") for v in ta.values()) and any(v=="manipulation" for v in ta.values()), ta)

    print("\n### J. long-press / text selection on cards")
    sel = pg.evaluate("""() => {
        const c=document.querySelector('.bubble-content');
        return c ? {userSelect: getComputedStyle(c).userSelect, webkitUserSelect: getComputedStyle(c).webkitUserSelect} : null;
    }""")
    print("      ", sel)

    print("\n### K. -webkit-tap-highlight-color (blue flash)")
    hl = pg.evaluate("() => getComputedStyle(document.querySelector('#pushForm button[type=submit]')).webkitTapHighlightColor")
    ck("tap highlight disabled on primary buttons", "rgba(0, 0, 0, 0)" in hl, hl)

    print("\n### L. 100dvh vs iOS URL-bar collapse")
    vp = pg.evaluate("""() => {
        const c=document.querySelector('.container') || document.body;
        return {css: getComputedStyle(c).height, win: window.innerHeight,
                scrollable: document.documentElement.scrollHeight > window.innerHeight + 2};
    }""")
    print("      ", vp)

    print("\n### M. clipboard API availability (WebKit needs user gesture)")
    clip = pg.evaluate("() => ({hasClipboard: !!(navigator.clipboard && navigator.clipboard.writeText), secure: window.isSecureContext})")
    print("      ", clip)

    print("\n### N. image lightbox on a real upload")
    pg.set_input_files("#fileInput", {"name":"shot.png","mimeType":"image/png",
        "buffer": bytes.fromhex("89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c4890000000a49444154789c6300010000050001"+"0d0a2db4"+"0000000049454e44ae426082")})
    pg.wait_for_timeout(2500)
    imgs = pg.locator(".image-preview").count()
    ck("uploaded image renders a preview", imgs >= 1, f"{imgs} previews")
    if imgs:
        pg.locator(".image-preview").first.click()
        pg.wait_for_timeout(600)
        ck("lightbox opens on tap", pg.locator("#lightbox.show").count() == 1)
        pg.click("#lightbox")
        pg.wait_for_timeout(500)
        ck("lightbox closes on tap", pg.locator("#lightbox.show").count() == 0)

    print("\n### O. rapid double-tap on Send must not double-post")
    before = pg.evaluate("() => document.querySelectorAll('.message-card').length")
    pg.fill("#textInput", "doubletap-probe")
    pg.evaluate("""() => {
        const f=document.getElementById('pushForm');
        const b=f.querySelector('button[type=submit]');
        b.click(); b.click(); b.click();
    }""")
    pg.wait_for_timeout(3000)
    after = pg.evaluate("() => document.querySelectorAll('.message-card').length")
    ck("triple-click Send creates at most 1 message", after - before <= 1, f"{before} -> {after}")

    print("\n### P. JS errors overall")
    ck("no page errors during whole session", not errs, errs[:5])

    b.close()

bad=[r for r in res if not r[1]]
print(f"\n{'ALL PASS' if not bad else 'FAILURES: '+str([x[0] for x in bad])}")
sys.exit(1 if bad else 0)
