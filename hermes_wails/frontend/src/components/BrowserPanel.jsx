import React, { useEffect, useRef, useState } from "react";

let proxyBase = "";

export default function BrowserPanel({ onClose }) {
  const [src, setSrc] = useState("about:blank");
  const [input, setInput] = useState("");
  const [status, setStatus] = useState("等待代理…");
  const frameRef = useRef(null);

  useEffect(() => {
    const onProxyReady = (url) => {
      proxyBase = url.replace(/\/welcome$/, "");
      setStatus("就绪");
      if (src === "about:blank") setSrc(url);
    };
    const onNavigate = (url) => {
      if (typeof url === "string" && url) {
        setSrc(url);
        setStatus("加载中…");
      }
    };
    const onOpen = () => {};

    window.runtime.EventsOn("proxy_ready", onProxyReady);
    window.runtime.EventsOn("browser_navigate", onNavigate);
    window.runtime.EventsOn("browser_open", onOpen);

    const onCmd = (cmd) => {
      const { id, action, arg } = cmd;
      const frame = frameRef.current;
      if (!frame || !frame.contentWindow) {
        callResult(id, "browser not ready");
        return;
      }
      const msg = { __hId: id };
      if (action === "eval") { msg.__hAct = "eval"; msg.__hJs = arg; }
      else if (action === "click") { msg.__hAct = "click"; msg.__hSel = arg; }
      else if (action === "text") { msg.__hAct = "text"; }
      else if (action === "html") { msg.__hAct = "html"; }

      const handler = (e) => {
        if (e.data && e.data.__hId === id) {
          window.removeEventListener("message", handler);
          callResult(id, e.data.__hRes || "");
        }
      };
      const timer = setTimeout(() => {
        window.removeEventListener("message", handler);
        callResult(id, "timeout");
      }, 9000);
      window.addEventListener("message", handler);
      try { frame.contentWindow.postMessage(msg, "*"); }
      catch (err) {
        clearTimeout(timer);
        window.removeEventListener("message", handler);
        callResult(id, "postMessage error: " + err.message);
      }
    };

    window.runtime.EventsOn("browser_cmd", onCmd);
    return () => {
      window.runtime.EventsOff("browser_navigate", onNavigate);
      window.runtime.EventsOff("browser_cmd", onCmd);
    };
  }, [src]);

  const go = () => {
    let u = input.trim();
    if (!u) return;
    if (!/^https?:\/\//.test(u)) u = "https://" + u;
    const dest = proxyBase
      ? proxyBase + "/?url=" + encodeURIComponent(u)
      : u;
    setSrc(dest);
    setStatus("加载中…");
  };

  return (
    <div className="browser-panel">
      <div className="browser-bar">
        <button className="ghost" onClick={onClose}>✕</button>
        <input
          value={input}
          placeholder={proxyBase ? "e.g. example.com" : "代理未启动…"}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && go()}
        />
        <button className="primary" onClick={go}>前往</button>
      </div>
      <div className="browser-status">{status}</div>
      <iframe
        ref={frameRef}
        className="browser-frame"
        src={src}
        title="browser"
        onLoad={() => {
          setStatus("页面已加载");
          if (window.go && window.go.main && window.go.main.App) {
            window.go.main.App.BrowserResult("__ready__", "loaded");
          }
        }}
      />
    </div>
  );
}

function callResult(id, result) {
  if (window.go && window.go.main && window.go.main.App) {
    window.go.main.App.BrowserResult(String(id), String(result));
  }
}
