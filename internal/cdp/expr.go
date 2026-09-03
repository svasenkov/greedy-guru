package cdp

import (
	"encoding/json"
	"strconv"
)

// DefaultTimeoutMS matches living Playwright actionTimeout / expect (5s).
// Mill wait, text, navigate ready, and Page.navigate use this when IR omits timeout_ms.
const DefaultTimeoutMS = 5000

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func WaitExpr(sel string) string {
	return `document.querySelector(` + jsString(sel) + `) != null`
}

func WaitPromiseExpr(sel string, timeoutMS int) string {
	return waitUntilJS(`document.querySelector(`+jsString(sel)+`) != null`, timeoutMS, DefaultTimeoutMS)
}

func TextPromiseExpr(sel, want string, timeoutMS int) string {
	check := `(() => {
  const el = document.querySelector(` + jsString(sel) + `);
  if (!el) return false;
  const t = (el.innerText || el.textContent || "");
  return t.includes(` + jsString(want) + `);
})()`
	return waitUntilJS(check, timeoutMS, DefaultTimeoutMS)
}

func ReadyPromiseExpr(timeoutMS int) string {
	return waitUntilJS(
		`document.readyState === "complete" || document.readyState === "interactive"`,
		timeoutMS,
		DefaultTimeoutMS,
	)
}

func waitUntilJS(check string, timeoutMS, fallback int) string {
	if timeoutMS <= 0 {
		timeoutMS = fallback
	}
	return `(function() {
  const hit = () => (` + check + `);
  if (hit()) return true;
  const timeoutMS = ` + strconv.Itoa(timeoutMS) + `;
  return new Promise((resolve, reject) => {
    let done = false;
    const finish = (ok, err) => {
      if (done) return;
      done = true;
      try { obs.disconnect(); } catch (e) {}
      clearTimeout(timer);
      if (ok) resolve(true);
      else reject(err || new Error("timeout"));
    };
    const obs = new MutationObserver(() => { if (hit()) finish(true); });
    obs.observe(document.documentElement, { subtree: true, childList: true, attributes: true, characterData: true });
    const timer = setTimeout(() => finish(false, new Error("timeout")), timeoutMS);
    const raf = () => {
      if (done) return;
      if (hit()) { finish(true); return; }
      requestAnimationFrame(raf);
    };
    requestAnimationFrame(raf);
    document.addEventListener("readystatechange", () => { if (hit()) finish(true); });
  });
})()`
}

func ClickExpr(sel string) string {
	return `(() => {
  const el = document.querySelector(` + jsString(sel) + `);
  if (!el) throw new Error("missing " + ` + jsString(sel) + `);
  el.click();
  return true;
})()`
}

func FillExpr(sel, value string) string {
	return `(() => {
  const el = document.querySelector(` + jsString(sel) + `);
  if (!el) throw new Error("missing " + ` + jsString(sel) + `);
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const desc = Object.getOwnPropertyDescriptor(proto, "value");
  if (desc && desc.set) desc.set.call(el, ` + jsString(value) + `);
  else el.value = ` + jsString(value) + `;
  el.dispatchEvent(new Event("input", { bubbles: true }));
  el.dispatchEvent(new Event("change", { bubbles: true }));
  return true;
})()`
}

func TextExpr(sel, want string) string {
	return `(() => {
  const el = document.querySelector(` + jsString(sel) + `);
  if (!el) throw new Error("missing " + ` + jsString(sel) + `);
  const t = (el.innerText || el.textContent || "");
  if (!t.includes(` + jsString(want) + `)) throw new Error("text want " + ` + jsString(want) + ` + " got " + t);
  return true;
})()`
}
