import React, { useState } from "react";

export default function InputBar({ onSend, disabled }) {
  const [text, setText] = useState("");

  const submit = () => {
    if (!text.trim() || disabled) return;
    onSend(text);
    setText("");
  };

  const onKeyDown = (e) => {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      submit();
    }
  };

  return (
    <div className="inputbar">
      <textarea
        value={text}
        placeholder="描述你的问题…  (Ctrl+Enter 发送)"
        onChange={(e) => setText(e.target.value)}
        onKeyDown={onKeyDown}
        rows={2}
        disabled={disabled}
      />
      <button className="send" onClick={submit} disabled={disabled || !text.trim()}>
        发送 ▶
      </button>
    </div>
  );
}
