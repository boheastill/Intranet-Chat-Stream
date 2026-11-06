#!/usr/bin/env python3
"""iOS-specific edge cases: keyboard, rotation, bfcache, storage, autofill."""
import sys
from playwright.sync_api import sync_playwright
import os
BASE=os.environ.get("ICS_TEST_URL","http://127.0.0.1:8666")
PW=os.environ.get("ICS_TEST_PASSWORD","66666666")
KEY=os.environ.get("ICS_TEST_KEY","vip")
res=[]
def ck(n,c,d=""):
    res.append((n,bool(c),d)); print(("  PASS  " if c else "  FAIL  ")+n+(f"  -> {d}" if d and not c else ""))

with sync_playwright() as p:
    b=p.webkit.launch(headless=True)
    ctx=b.new_context(**p.devices["iPad (gen 7)"]); pg=ctx.new_page()
    errs=[]; pg.on("pageerror", lambda e: errs.append(str(e)[:200]))
    pg.goto(f"{BASE}/?key={KEY}", wait_until="load"); pg.wait_for_timeout(500)

    print("\n### Q. autocomplete/autocorrect attributes on secret fields")
    at = pg.evaluate("""() => {
        const f=document.getElementById('passwordField');
        return {type:f.type, autocomplete:f.autocomplete, autocorrect:f.getAttribute('autocorrect'),
                autocapitalize:f.getAttribute('autocapitalize'), spellcheck:f.spellcheck,
                inputmode:f.getAttribute('inputmode')};
    }""")
    ck("password field disables autofill/autocorrect",
       at["autocomplete"] in ("off","new-password") and at["autocorrect"]=="off"
       and at["autocapitalize"]=="off" and at["spellcheck"] is False, at)

    print("\n### R. Enter key submits from the password field")
    pg.fill("#passwordField", PW)
    pg.press("#passwordField", "Enter")
    pg.wait_for_timeout(1800)
    ck("Enter submits login", bool(pg.evaluate("()=>localStorage.getItem('ics_token')")))

    print("\n### S. rotation to landscape keeps layout sane")
    pg.set_viewport_size({"width":1080,"height":810})
    pg.wait_for_timeout(500)
    ow = pg.evaluate("() => document.documentElement.scrollWidth - document.documentElement.clientWidth")
    ck("landscape: no horizontal overflow", ow <= 1, f"{ow}px")
    pg.set_viewport_size({"width":810,"height":1080})
    pg.wait_for_timeout(300)

    print("\n### T. bfcache restore (iOS back/forward)")
    pg.goto(f"{BASE}/?key={KEY}", wait_until="load"); pg.wait_for_timeout(300)
    pg.go_back(); pg.wait_for_timeout(400)
    pg.go_forward(); pg.wait_for_timeout(1200)
    ck("page still functional after back/forward",
       pg.evaluate("() => typeof window.icsLoginSubmit === 'function' && typeof initAuth === 'function'"))

    print("\n### U. localStorage blocked (iOS private mode / cookie block)")
    b2=p.webkit.launch(headless=True); ctx2=b2.new_context(**p.devices["iPad (gen 7)"]); pg2=ctx2.new_page()
    e2=[]; pg2.on("pageerror", lambda e: e2.append(str(e)[:200]))
    pg2.add_init_script("""
        Object.defineProperty(window, 'localStorage', {
            get() { throw new DOMException('denied', 'SecurityError'); }
        });
    """)
    pg2.goto(f"{BASE}/?key={KEY}", wait_until="load"); pg2.wait_for_timeout(800)
    ck("survives blocked localStorage (no crash)", not e2, e2[:3])
    ck("login modal still usable with storage blocked", pg2.locator("#loginModal").is_visible())
    b2.close()

    print("\n### V. fetch failure shows a message (offline)")
    ctx3=b.new_context(**p.devices["iPad (gen 7)"]); pg3=ctx3.new_page()
    pg3.goto(f"{BASE}/?key={KEY}", wait_until="load"); pg3.wait_for_timeout(400)
    pg3.route("**/api/login", lambda r: r.abort())
    pg3.fill("#passwordField", PW); pg3.click("#btnLogin"); pg3.wait_for_timeout(1500)
    shown = pg3.locator("#loginError").is_visible()
    txt = pg3.locator("#loginError").inner_text() if shown else ""
    ck("network failure shows an error to the user", shown and len(txt) > 0, repr(txt))
    ck("button re-enabled after failed request", pg3.evaluate("()=>!document.getElementById('btnLogin').disabled"))

    print("\n### W. no JS errors overall")
    ck("no page errors", not errs, errs[:5])
    b.close()

bad=[r for r in res if not r[1]]
print(f"\n{'ALL PASS' if not bad else 'FAILURES: '+str([x[0] for x in bad])}")
sys.exit(1 if bad else 0)
