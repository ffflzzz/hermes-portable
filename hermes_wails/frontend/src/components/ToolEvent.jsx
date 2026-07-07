import React, { useState } from "react";

const META = {
  tool_start: { icon: "💭", label: "思考", cls: "think" },
  tool_call: { icon: "🔧", label: "调用工具", cls: "call" },
  tool_result: { icon: "📤", label: "工具输出", cls: "result" },
};

function ToolCallCard({ call, result }) {
  const [open, setOpen] = useState(false);
  const cmd = call?.payload || "";
  const out = result?.payload || "";
  const nameMatch = cmd.match(/^(\w+):/);
  const toolName = nameMatch ? nameMatch[1] : "tool";
  const argText = nameMatch ? cmd.slice(nameMatch[0].length).trim() : cmd;
  const isChrome = toolName === "chrome_launch" || toolName === "chrome_cdp";

  return (
    <div className={`tool-card ${isChrome ? "chrome" : ""}`}>
      <button className="tool-card-head" onClick={() => setOpen((o) => !o)}>
        <span className="tool-status running" />
        <span className="tool-card-icon">{isChrome ? "🌐" : "🔧"}</span>
        <span className="tool-card-name">{toolName}</span>
        <span className="tool-card-cmd">{argText}</span>
        <span className="tool-card-chevron">{open ? "▾" : "▸"}</span>
      </button>
      {open && (
        <div className="tool-card-body">
          {out && (
            <pre className="tool-output">{out.length > 4000 ? out.slice(0, 4000) + "\n…(truncated)" : out}</pre>
          )}
        </div>
      )}
    </div>
  );
}

export default function ToolEvent({ data }) {
  // data is a single event; we group call+result in App via state.
  // Here we render compact rows for non-grouped fallback.
  const m = META[data.kind] || { icon: "•", label: data.kind, cls: "" };
  return (
    <div className={`tool-row ${m.cls}`}>
      <span className="tool-row-icon">{m.icon}</span>
      <span className="tool-row-label">{m.label}</span>
      <span className="tool-row-payload">{data.payload}</span>
    </div>
  );
}

export { ToolCallCard };
