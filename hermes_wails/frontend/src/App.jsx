import React, { useEffect, useRef, useState, useCallback } from "react";
import ChatBubble from "./components/ChatBubble.jsx";
import InputBar from "./components/InputBar.jsx";
import ApiKeyModal from "./components/ApiKeyModal.jsx";
import ToolEvent, { ToolCallCard } from "./components/ToolEvent.jsx";
import BrowserPanel from "./components/BrowserPanel.jsx";

// Light cleanup of residual Markdown so replies read like plain chat text.
function cleanReply(text) {
  return text
    .split("\n")
    .map((line) => {
      let s = line;
      // strip heading marks
      s = s.replace(/^#{1,6}\s+/, "");
      // strip bold/italic markers
      s = s.replace(/\*\*([^*]+)\*\*/g, "$1");
      s = s.replace(/\*([^*]+)\*/g, "$1");
      s = s.replace(/__([^_]+)__/g, "$1");
      // strip blockquote marks
      s = s.replace(/^>\s?/, "");
      // strip list markers (keep the text)
      s = s.replace(/^\s*[-*+]\s+/, "");
      s = s.replace(/^\s*\d+\.\s+/, "");
      // strip code fence lines
      s = s.replace(/^```.*$/, "");
      return s;
    })
    .join("\n");
}

export default function App() {
  const [messages, setMessages] = useState([]);
  const [toolCards, setToolCards] = useState([]); // grouped call+result cards
  const [status, setStatus] = useState("ready"); // ready | thinking
  const [showKeyModal, setShowKeyModal] = useState(false);
  const [busy, setBusy] = useState(false);
  const [showBrowser, setShowBrowser] = useState(false);
  const scrollRef = useRef(null);
  const eventsBound = useRef(false);

  const appendMessage = useCallback((role, content) => {
    setMessages((m) => [...m, { id: Date.now() + Math.random(), role, content }]);
  }, []);

  useEffect(() => {
    if (eventsBound.current) return;
    eventsBound.current = true;

    const onUser = (msg) => appendMessage("user", msg);
    const onAssistant = (msg) => {
      setToolCards([]);
      setMessages((m) => {
        const next = m.slice();
        const last = next[next.length - 1];
        if (last && last.role === "assistant" && last.streaming) {
          last.content = cleanReply(last.content);
          delete last.streaming;
          return [...next];
        }
        return [...next, { id: Date.now() + Math.random(), role: "assistant", content: cleanReply(msg) }];
      });
    };
    const onStatus = (s) => {
      setStatus(s);
      setBusy(s === "thinking");
      if (s === "ready") setToolCards([]);
    };
    const onToken = (tok) => {
      setMessages((m) => {
        const next = m.slice();
        const last = next[next.length - 1];
        if (last && last.role === "assistant" && last.streaming) {
          last.content += tok;
          return [...next];
        }
        return [...next, { id: Date.now() + Math.random(), role: "assistant", content: tok, streaming: true }];
      });
    };
    const onHistory = (hist) => {
      if (Array.isArray(hist) && hist.length) {
        setMessages(hist.map((m) => ({ ...m, id: Date.now() + Math.random() })));
      }
    };
    const onTool = (e) => {
      const ev = { ...e, id: Date.now() + Math.random() };
      setToolCards((cards) => {
        if (ev.kind === "tool_call") {
          return [...cards, { id: ev.id, call: ev, result: null }];
        }
        if (ev.kind === "tool_result") {
          // attach to the last card without a result
          const next = cards.slice();
          for (let i = next.length - 1; i >= 0; i--) {
            if (next[i].result === null) {
              next[i] = { ...next[i], result: ev };
              break;
            }
          }
          return next;
        }
        return cards;
      });
    };
    const onNeedKey = (need) => setShowKeyModal(!!need);

    window.runtime.EventsOn("user_msg", onUser);
    window.runtime.EventsOn("assistant_msg", onAssistant);
    window.runtime.EventsOn("token", onToken);
    window.runtime.EventsOn("status", onStatus);
    window.runtime.EventsOn("tool_event", onTool);
    window.runtime.EventsOn("history", onHistory);
    window.runtime.EventsOn("need_apikey", onNeedKey);

    window.go.main.App.GetAPIKeyStatus().then((ok) => {
      if (!ok) setShowKeyModal(true);
    });
    window.go.main.App.LoadHistory().then((hist) => {
      if (Array.isArray(hist) && hist.length) {
        setMessages(hist.map((m) => ({ ...m, id: Date.now() + Math.random() })));
      }
    });
  }, [appendMessage]);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages, toolCards]);

  const send = useCallback(
    (text) => {
      if (busy || !text.trim()) return;
      setToolCards([]);
      window.go.main.App.SendMessage(text);
    },
    [busy]
  );

  const saveKey = useCallback((key) => {
    window.go.main.App.SetAPIKey(key);
    setShowKeyModal(false);
  }, []);

  const clearChat = useCallback(() => {
    window.go.main.App.ClearHistory();
    setMessages([]);
    setToolCards([]);
  }, []);

  return (
    <div className="app">
      <div className="app-main">
        <header className="topbar">
          <div className="brand">
            <span className="logo">🤖</span>
            <span className="title">Hermes 诊断助手</span>
            <span className="sub">Portable Edition</span>
          </div>
          <div className="actions">
            <button className="ghost" onClick={clearChat}>清空</button>
            <button className="ghost" onClick={() => setShowBrowser((s) => !s)}>🌐 浏览器</button>
            <button className="ghost" onClick={() => setShowKeyModal(true)}>设置</button>
            <span className={`status-dot ${status}`} />
            <span className="status-text">
              {status === "thinking" ? "思考中…" : "就绪"}
            </span>
          </div>
        </header>

        <main className="chat" ref={scrollRef}>
          {messages.length === 0 && (
            <div className="welcome">
              <div className="welcome-logo">🤖</div>
              <h2>欢迎使用 Hermes 诊断助手</h2>
              <p>插上即用，帮你诊断并修复电脑问题。</p>
              <p className="hint">直接输入问题，例如「C盘空间不足怎么办？」</p>
            </div>
          )}

          {messages.map((m) =>
            m.role === "tool" ? (
              <ToolEvent key={m.id} data={m} />
            ) : (
              <ChatBubble key={m.id} role={m.role} content={m.content} streaming={m.streaming} />
            )
          )}

          {toolCards.length > 0 && (
            <div className="tool-tray">
              {toolCards.map((c) => (
                <ToolCallCard key={c.id} call={c.call} result={c.result} />
              ))}
            </div>
          )}

          {status === "thinking" && (
            <div className="typing">
              <span></span><span></span><span></span>
            </div>
          )}
        </main>

        <InputBar onSend={send} disabled={busy} />
      </div>

      {showBrowser && (
        <div className="browser-dock">
          <BrowserPanel />
        </div>
      )}

      {showKeyModal && (
        <ApiKeyModal onSave={saveKey} onClose={() => setShowKeyModal(false)} />
      )}
    </div>
  );
}
