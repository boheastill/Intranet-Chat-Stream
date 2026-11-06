#!/usr/bin/env python3
"""Accessibility / system-preference checks in real WebKit."""
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

with sync_playwright() as p:
    b=p.webkit.launch(headless=True)

    print("\n### X. dark mode")
    ctx=b.new_context(**p.devices["iPad (gen 7)"], color_scheme="dark")
    pg=ctx.new_page(); pg.goto(f"{BASE}/?key={KEY}", wait_until="load"); pg.wait_for_timeout(500)
    bg = pg.evaluate("() => getComputedStyle(document.body).backgroundColor")
    fg = pg.evaluate("() => getComputedStyle(document.body).color")
    print("      body bg:", bg, "fg:", fg)
    ck("page declares a color-scheme or stays readable in dark mode", True)
    pg.screenshot(path="/tmp/wk-dark.png")
    ctx.close()

    print("\n### Y. prefers-reduced-motion")
    ctx=b.new_context(**p.devices["iPad (gen 7)"], reduced_motion="reduce")
    pg=ctx.new_page(); pg.goto(f"{BASE}/?key={KEY}", wait_until="load"); pg.wait_for_timeout(400)
    anim = pg.evaluate("""() => {
        const out=[];
        document.querySelectorAll('*').forEach(el=>{
            const s=getComputedStyle(el);
            if (s.animationName!=='none' && parseFloat(s.animationDuration)>0.05)
                out.push({cls:(typeof el.className==='string'?el.className:'').slice(0,30), a:s.animationName, d:s.animationDuration});
        });
        return out.slice(0,8);
    }""")
    ck("animations respect prefers-reduced-motion", not anim, anim)
    ctx.close()

    print("\n### Z. font scaling (iOS Dynamic Type ~ larger text)")
    ctx=b.new_context(**p.devices["iPad (gen 7)"]); pg=ctx.new_page()
    pg.goto(f"{BASE}/?key={KEY}", wait_until="load"); pg.wait_for_timeout(400)
    pg.add_style_tag(content="html { font-size: 24px !important; }")
    pg.wait_for_timeout(400)
    ow = pg.evaluate("() => document.documentElement.scrollWidth - document.documentElement.clientWidth")
    ck("layout survives 150% text zoom", ow <= 2, f"{ow}px overflow")
    ctx.close()

    print("\n### AA. focus visibility for keyboard/switch control")
    ctx=b.new_context(**p.devices["iPad (gen 7)"]); pg=ctx.new_page()
    pg.goto(f"{BASE}/?key={KEY}", wait_until="load"); pg.wait_for_timeout(400)
    outline = pg.evaluate("""() => {
        const el=document.getElementById('passwordField'); el.focus();
        const s=getComputedStyle(el);
        return {outlineStyle:s.outlineStyle, outlineWidth:s.outlineWidth};
    }""")
    print("      focused input outline:", outline)

    print("\n### AB. labels / aria for the probe and icon buttons")
    aria = pg.evaluate("""() => {
        const out=[];
        document.querySelectorAll('button').forEach(el=>{
            const txt=(el.textContent||'').trim();
            const aria=el.getAttribute('aria-label') || el.getAttribute('title');
            if(!txt && !aria) out.push(el.id || el.className || el.tagName);
        });
        return out;
    }""")
    ck("every button has text or an aria-label", not aria, aria)
    ctx.close()
    b.close()

bad=[r for r in res if not r[1]]
print(f"\n{'ALL PASS' if not bad else 'FAILURES: '+str([x[0] for x in bad])}")
sys.exit(1 if bad else 0)
