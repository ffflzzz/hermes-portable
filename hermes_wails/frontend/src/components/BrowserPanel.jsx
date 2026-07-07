import React, { useState } from "react";

// Visual-only browser preview. The AI "navigates" via the backend tool
// browser_navigate, which emits a browser_navigate event to update this panel.
// Page content reading is done server-side by the backend (browser_read),
// so it works for any URL without cross-origin scripting limits.

export default function BrowserPanel() {
  const [url, setUrl] = useState("about:blank");
  const [input, setInput] = useState("");

  // listen for AI-driven navigation
  React.useEffect(() => {
    const handler = (u) => {
      if (typeof u === "string" && u) setUrl(u);
    };
    window.runtime.EventsOn("browser_navigate", handler);
    return () => window.runtime.EventsOff("browser_navigate", handler);
  }, []);

  const go = () => {
    let u = input.trim();
    if (!u) return;
    if (!/^https?:\/\//.test(u)) u = "https://" + u;
    setUrl(u);
  };

  return (
    <div className="browser-panel">
      <div className="browser-bar">
        <button className="ghost" onClick={() => setUrl("about:blank")}>✕</button>
        <input
          value={input}
          placeholder="输入网址，如 example.com"
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && go()}
        />
        <button className="primary" onClick={go}>前往</button>
      </div>
      <iframe
        className="browser-frame"
        src={url}
        title="browser"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
      />
    </div>
  );
}
