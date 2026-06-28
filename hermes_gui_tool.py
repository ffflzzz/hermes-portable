#!/usr/bin/env python3
"""
Hermes Agent Lite - GUI Edition
A minimal Hermes Agent with GUI + full tool support (terminal, read_file, write_file).
Packaged as a single executable with PyInstaller.
"""

import os
import sys
import json
import time
import subprocess
import platform
import threading
from pathlib import Path

try:
    import tkinter as tk
    from tkinter import ttk, messagebox, scrolledtext
except ImportError:
    sys.exit("ERROR: tkinter not available")

# ─── Configuration ───────────────────────────────────────────────────────
API_BASE = "https://apihub.agnes-ai.com/v1"
DEFAULT_MODEL = "agnes-2.0-flash"
SCRIPT_DIR = Path(__file__).parent.resolve()
CONFIG_FILE = SCRIPT_DIR / "config.json"
SESSION_DIR = SCRIPT_DIR / ".sessions"


# ─── Config Management ──────────────────────────────────────────────────
def load_config():
    if CONFIG_FILE.exists():
        try:
            with open(CONFIG_FILE, "r", encoding="utf-8") as f:
                return json.load(f)
        except:
            pass
    return {}


def save_config(cfg):
    CONFIG_FILE.write_text(json.dumps(cfg, ensure_ascii=False, indent=2))


# ─── Session Management ─────────────────────────────────────────────────
def get_session_path():
    SESSION_DIR.mkdir(parents=True, exist_ok=True)
    return SESSION_DIR / "default.json"


def load_session():
    p = get_session_path()
    if p.exists():
        try:
            with open(p, "r", encoding="utf-8") as f:
                return json.load(f)
        except:
            pass
    return {"messages": [], "created": time.time()}


def save_session(sess):
    with open(get_session_path(), "w", encoding="utf-8") as f:
        json.dump(sess, f, indent=2, ensure_ascii=False)


# ─── System Info ─────────────────────────────────────────────────────────
def collect_sys_info():
    info = []
    info.append(f"OS: {platform.system()} {platform.release()} ({platform.machine()})")
    info.append(f"Hostname: {platform.node()}")
    info.append(f"CPU Cores: {os.cpu_count()}")
    try:
        if platform.system() == "Windows":
            out = subprocess.check_output("systeminfo", shell=True, text=True, timeout=30)
            info.append(out[:1000])
        else:
            out = subprocess.check_output("uname -a", shell=True, text=True)
            info.append(out.strip())
            out = subprocess.check_output("df -h", shell=True, text=True)
            info.append(out.strip())
    except:
        pass
    return "\n".join(info)


# ─── Tool Execution ──────────────────────────────────────────────────────
def tool_terminal(command):
    """Execute a shell command."""
    try:
        if platform.system() == "Windows":
            proc = subprocess.run(
                ["cmd", "/C", command],
                capture_output=True, text=True, timeout=60
            )
        else:
            proc = subprocess.run(
                ["bash", "-c", command],
                capture_output=True, text=True, timeout=60
            )
        result = proc.stdout + proc.stderr
        if proc.returncode != 0:
            result += f"\n[Exit code: {proc.returncode}]"
        return json.dumps({"success": True, "output": result[:50000]})
    except subprocess.TimeoutExpired:
        return json.dumps({"success": False, "error": "Command timed out"})
    except Exception as e:
        return json.dumps({"success": False, "error": str(e)})


def tool_read_file(path):
    """Read file contents."""
    try:
        if not os.path.isabs(path):
            path = os.path.join(SCRIPT_DIR, path)
        with open(path, "r", encoding="utf-8") as f:
            content = f.read()
        return json.dumps({"success": True, "content": content[:50000], "lines": len(content.splitlines())})
    except Exception as e:
        return json.dumps({"success": False, "error": str(e)})


def tool_write_file(path, content):
    """Write content to a file."""
    try:
        if not os.path.isabs(path):
            path = os.path.join(SCRIPT_DIR, path)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            f.write(content)
        return json.dumps({"success": True, "bytes_written": len(content)})
    except Exception as e:
        return json.dumps({"success": False, "error": str(e)})


# Tool definitions for API
TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "terminal",
            "description": "Execute a shell command. Returns stdout and stderr.",
            "parameters": {
                "type": "object",
                "properties": {
                    "command": {
                        "type": "string",
                        "description": "The shell command to execute"
                    }
                },
                "required": ["command"]
            }
        }
    },
    {
        "type": "function",
        "function": {
            "name": "read_file",
            "description": "Read the contents of a file.",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {
                        "type": "string",
                        "description": "File path to read"
                    }
                },
                "required": ["path"]
            }
        }
    },
    {
        "type": "function",
        "function": {
            "name": "write_file",
            "description": "Write content to a file.",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {
                        "type": "string",
                        "description": "File path to write"
                    },
                    "content": {
                        "type": "string",
                        "description": "Content to write"
                    }
                },
                "required": ["path", "content"]
            }
        }
    }
]

TOOL_HANDLERS = {
    "terminal": tool_terminal,
    "read_file": tool_read_file,
    "write_file": tool_write_file,
}


# ─── API Call ────────────────────────────────────────────────────────────
def call_api(messages, api_key, tools=None, max_turns=10):
    """Call the LLM API with tool calling support."""
    system_msg = {"role": "system", "content": """You are Hermes Agent Lite, a diagnostic AI assistant with full system access.

Available tools:
- terminal: Execute shell commands. Parameter: "command" (string)
- read_file: Read file contents. Parameter: "path" (string)
- write_file: Write content to file. Parameters: "path" (string), "content" (string)

When the user asks a question:
1. Think about what tools you need
2. Call tools as needed
3. Return clear, actionable results
4. For non-technical users, explain in simple terms

Always use the local system's shell. Be thorough but concise."""}

    all_messages = [system_msg] + messages

    for turn in range(max_turns):
        payload = {
            "model": DEFAULT_MODEL,
            "messages": all_messages,
            "temperature": 0.7,
            "max_tokens": 8192,
            "top_p": 0.9,
        }
        if tools:
            payload["tools"] = tools

        try:
            import requests
            r = requests.post(
                f"{API_BASE}/chat/completions",
                json=payload,
                headers={
                    "Authorization": f"Bearer {api_key}",
                    "Content-Type": "application/json"
                },
                timeout=120
            )

            if r.status_code == 200:
                resp = r.json()
                choice = resp["choices"][0]

                # Check for tool calls
                if choice.get("message", {}).get("tool_calls"):
                    tool_calls = choice["message"]["tool_calls"]
                    assistant_msg = choice["message"]

                    # Append assistant message
                    all_messages.append(assistant_msg)

                    # Execute each tool call
                    for tc in tool_calls:
                        func_name = tc["function"]["name"]
                        func_args = json.loads(tc["function"]["arguments"])
                        tc_id = tc["id"]

                        # Get handler
                        handler = TOOL_HANDLERS.get(func_name)
                        if handler:
                            try:
                                result = handler(**func_args)
                            except Exception as e:
                                result = json.dumps({"success": False, "error": str(e)})
                        else:
                            result = json.dumps({"success": False, "error": f"Unknown tool: {func_name}"})

                        # Append tool result
                        all_messages.append({
                            "role": "tool",
                            "tool_call_id": tc_id,
                            "content": result
                        })

                    # Continue loop for next turn
                    continue

                # Text response
                assistant_msg = choice["message"]
                all_messages.append(assistant_msg)
                return assistant_msg.get("content", ""), all_messages

            elif r.status_code == 401:
                return "❌ API Key无效", all_messages
            elif r.status_code == 429:
                return "⏳ 请求太频繁，请稍后再试", all_messages
            else:
                return f"❌ HTTP {r.status_code}", all_messages

        except Exception as e:
            return f"❌ 网络错误: {e}", all_messages

    return "⚠️ 达到最大轮次限制", all_messages


# ─── GUI ─────────────────────────────────────────────────────────────────
class ChatApp:
    def __init__(self, root, api_key):
        self.root = root
        self.api_key = api_key
        self.session = load_session()
        self.busy = False

        self.root.title("Hermes Agent Lite v1.0")
        self.root.geometry("800x600")

        # Theme
        style = ttk.Style()
        style.theme_use('clam')
        style.configure('.', font=('Microsoft YaHei UI', 10))

        # Top bar
        top_bar = ttk.Frame(root)
        top_bar.pack(fill='x', padx=5, pady=2)

        self.status_label = ttk.Label(top_bar, text="✅ 就绪", foreground='#2ecc71')
        self.status_label.pack(side='right')

        ttk.Button(top_bar, text="清空对话", command=self.clear_chat,
                   style='Accent.TButton').pack(side='left')

        # Chat area
        chat_frame = ttk.Frame(root)
        chat_frame.pack(fill='both', expand=True, padx=5, pady=5)

        self.chat_text = scrolledtext.ScrolledText(
            chat_frame, wrap='word', state='disabled',
            bg='#1a1a2e', fg='#e0e0e0',
            font=('Consolas', 10),
            relief='flat',
            borderwidth=0
        )
        self.chat_text.pack(fill='both', expand=True)

        # Style chat text
        self.chat_text.tag_config("user", foreground='#2ecc71')
        self.chat_text.tag_config("assistant", foreground='#3498db')
        self.chat_text.tag_config("tool", foreground='#e67e22')
        self.chat_text.tag_config("separator", foreground='#555')

        # Input area
        input_frame = ttk.Frame(root)
        input_frame.pack(fill='x', padx=5, pady=(0, 5))

        self.input_entry = ttk.Entry(input_frame)
        self.input_entry.pack(side='left', fill='x', expand=True, padx=(0, 5))
        self.input_entry.bind('<Return>', lambda e: self.send_message())

        self.send_btn = ttk.Button(input_frame, text="发送", command=self.send_message)
        self.send_btn.pack(side='right')

        # Welcome message
        self.append_chat("assistant", "欢迎使用 Hermes Agent Lite v1.0\n\n我是你的AI诊断助手，可以帮你：\n• 执行系统命令\n• 读写文件\n• 分析系统问题\n\n直接输入问题即可开始对话。")

        # Auto system info on first run
        if len(self.session['messages']) == 0:
            self.append_chat("tool", "📋 正在收集系统信息...")
            sys_info = collect_sys_info()
            self.append_chat("tool", sys_info)

            threading.Thread(target=self._auto_analyze, daemon=True).start()

    def _auto_analyze(self):
        user_msg = {"role": "user", "content": f"这是本机系统信息，请分析并给出初步诊断建议：\n{collect_sys_info()}"}
        self.session['messages'].append(user_msg)

        reply, msgs = call_api([user_msg], self.api_key, TOOLS, 10)
        self.session['messages'] = msgs
        save_session(self.session)

        self.root.after(0, lambda: self.append_chat("assistant", reply))
        self.root.after(0, lambda: self.status_label.config(text="✅ 就绪", foreground='#2ecc71'))

    def send_message(self):
        if self.busy:
            return
        text = self.input_entry.get().strip()
        if not text:
            return

        self.input_entry.delete(0, 'end')
        self.busy = True
        self.send_btn.config(state='disabled')
        self.status_label.config(text="⏳ 思考中...", foreground='#f39c12')

        self.append_chat("user", text)
        self.session['messages'].append({"role": "user", "content": text})

        threading.Thread(target=self._handle_response, args=(text,), daemon=True).start()

    def _handle_response(self, text):
        try:
            reply, msgs = call_api(self.session['messages'], self.api_key, TOOLS, 10)
            self.session['messages'] = msgs
            save_session(self.session)

            self.root.after(0, lambda: self.append_chat("assistant", reply))
            self.root.after(0, lambda: self.status_label.config(text="✅ 就绪", foreground='#2ecc71'))
        except Exception as e:
            self.root.after(0, lambda: self.append_chat("assistant", f"❌ 错误: {e}"))
            self.root.after(0, lambda: self.status_label.config(text="❌ 错误", foreground='#e74c3c'))
        finally:
            self.root.after(0, lambda: self._reset_ui())

    def _reset_ui(self):
        self.busy = False
        self.send_btn.config(state='normal')

    def append_chat(self, role, text):
        self.chat_text.config(state='normal')

        tag = role if role in ("user", "assistant", "tool") else "user"
        prefix = {
            "user": "👤 你: ",
            "assistant": "🤖 Hermes: ",
            "tool": "🔧 工具: "
        }.get(role, "")

        self.chat_text.insert('end', prefix, tag)
        self.chat_text.insert('end', text + "\n\n", tag)

        self.chat_text.see('end')
        self.chat_text.config(state='disabled')

    def clear_chat(self):
        self.session['messages'] = []
        save_session(self.session)
        self.chat_text.config(state='normal')
        self.chat_text.delete('1.0', 'end')
        self.append_chat("assistant", "对话已清空。")


def main():
    cfg = load_config()

    if not cfg.get("api_key"):
        root = tk.Tk()
        root.withdraw()
        api_key = simpledialog.askstring("API Key", "请输入你的 API Key:", parent=root)
        if not api_key:
            sys.exit(0)
        cfg["api_key"] = api_key
        save_config(cfg)

    api_key = cfg["api_key"]

    root = tk.Tk()
    app = ChatApp(root, api_key)
    root.mainloop()


if __name__ == "__main__":
    main()
