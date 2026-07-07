import React from "react";

export default function ChatBubble({ role, content }) {
  const isUser = role === "user";
  return (
    <div className={`bubble-row ${isUser ? "right" : "left"}`}>
      <div className="avatar">{isUser ? "👤" : "🤖"}</div>
      <div className={`bubble ${isUser ? "user" : "assistant"}`}>
        <div className="bubble-name">{isUser ? "你" : "Hermes"}</div>
        <div className="bubble-body">{content}</div>
      </div>
    </div>
  );
}
