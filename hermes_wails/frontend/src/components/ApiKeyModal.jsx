import React, { useState } from "react";

export default function ApiKeyModal({ onSave, onClose }) {
  const [key, setKey] = useState("");

  return (
    <div className="modal-mask" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <h3>🔑 设置 API Key</h3>
        <p className="modal-desc">输入你的 API Key，程序会安全保存在本地。</p>
        <input
          type="password"
          autoFocus
          placeholder="粘贴 API Key…"
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
        <div className="modal-actions">
          <button className="ghost" onClick={onClose}>取消</button>
          <button
            className="primary"
            disabled={!key.trim()}
            onClick={() => onSave(key.trim())}
          >
            保存
          </button>
        </div>
      </div>
    </div>
  );
}
