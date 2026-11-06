#!/usr/bin/env python3
"""Content robustness in real WebKit: long words, unicode, filenames, keyboard."""
import sys, json
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
    ctx=b.new_context(**p.devices["iPad (gen 7)"]); pg=ctx.new_page()
    errs=[]; pg.on("pageerror", lambda e: errs.append(str(e)[:200]))
    pg.goto(f"{BASE}/?key={KEY}", wait_until="load"); pg.wait_for_timeout(400)
    pg.fill("#passwordField", PW); login_with_retry(pg, PW)

    print("\n### AC. very long unbroken string must not blow out the layout")
    pg.fill("#textInput", "A"*400)
    pg.click("#pushForm button[type=submit]"); pg.wait_for_timeout(2200)
    ow = pg.evaluate("() => document.documentElement.scrollWidth - document.documentElement.clientWidth")
    ck("long unbroken text does not cause horizontal overflow", ow <= 2, f"{ow}px")
    if ow > 2:
        print("      offending:", pg.evaluate("""() => {
            const cw=document.documentElement.clientWidth, out=[];
            document.querySelectorAll('*').forEach(el=>{const r=el.getBoundingClientRect();
              if(r.right>cw+1) out.push({tag:el.tagName, cls:(typeof el.className==='string'?el.className:'').slice(0,30), right:Math.round(r.right)});});
            return out.slice(0,6);}"""))

    print("\n### AD. emoji / CJK / RTL text")
    pg.fill("#textInput", "中文测试 🎉 مرحبا 👍🏽")
    pg.click("#pushForm button[type=submit]"); pg.wait_for_timeout(2200)
    ow2 = pg.evaluate("() => document.documentElement.scrollWidth - document.documentElement.clientWidth")
    ck("mixed unicode does not overflow", ow2 <= 2, f"{ow2}px")
    ck("unicode text round-trips", "中文测试" in pg.inner_text("#timeline"))

    print("\n### AE. very long filename in a file card")
    pg.set_input_files("#fileInput", {"name":"a"*150 + ".txt", "mimeType":"text/plain", "buffer": b"hello"})
    pg.wait_for_timeout(2500)
    ow3 = pg.evaluate("() => document.documentElement.scrollWidth - document.documentElement.clientWidth")
    ck("long filename does not overflow", ow3 <= 2, f"{ow3}px")
    if ow3 > 2:
        print("      offending:", pg.evaluate("""() => {
            const cw=document.documentElement.clientWidth, out=[];
            document.querySelectorAll('*').forEach(el=>{const r=el.getBoundingClientRect();
              if(r.right>cw+1) out.push({tag:el.tagName, cls:(typeof el.className==='string'?el.className:'').slice(0,30), right:Math.round(r.right)});});
            return out.slice(0,6);}"""))

    print("\n### AF. iOS keyboard: composer stays reachable (visualViewport)")
    pg.focus("#textInput")
    pg.wait_for_timeout(300)
    vis = pg.evaluate("""() => {
        const el=document.getElementById('textInput');
        const r=el.getBoundingClientRect();
        return {top:Math.round(r.top), bottom:Math.round(r.bottom), winH:window.innerHeight,
                vvH: window.visualViewport ? Math.round(window.visualViewport.height) : null};
    }""")
    print("      ", vis)

    print("\n### AG. timeline scrolls independently, page itself does not")
    sc = pg.evaluate("""() => {
        const tl=document.querySelector('.timeline-container') || document.getElementById('timeline');
        return {pageScrollable: document.documentElement.scrollHeight > window.innerHeight + 2,
                timelineScrollable: tl.scrollHeight > tl.clientHeight + 2};
    }""")
    print("      ", sc)

    print("\n### AH. no JS errors")
    ck("no page errors", not errs, errs[:4])
    b.close()

bad=[r for r in res if not r[1]]
print(f"\n{'ALL PASS' if not bad else 'FAILURES: '+str([x[0] for x in bad])}")
sys.exit(1 if bad else 0)
