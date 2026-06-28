#!/usr/bin/env python3
"""
Hermes Diagnostic Client - Portable Edition v2.0
GUI + Auto-Diagnosis + Chat + Tool Support
Single executable, plug and play.
"""


import os
import sys
import json
import time
import threading
import subprocess
import webbrowser
import platform
import socket
from pathlib import Path

try:
    import requests
except ImportError:
    print("ERROR: requests library not found.")
    print("Install: python -m pip install requests")
    sys.exit(1)

try:
    import tkinter as tk
    from tkinter import ttk, messagebox, scrolledtext, simpledialog
except ImportError:
    sys.exit("ERROR: tkinter not available")

# ─── Configuration ───────────────────────────────────────────────
API_BASE = "https://apihub.agnes-ai.com/v1"
DEFAULT_MODEL = "agnes-2.0-flash"

SCRIPT_DIR = Path(__file__).parent.resolve()
DATA_DIR = SCRIPT_DIR / ".hermes_portable"
DATA_DIR.mkdir(exist_ok=True)

CONFIG_FILE = DATA_DIR / "config.json"
SESSION_FILE = DATA_DIR / "sessions" / "default.json"
SESSION_FILE.parent.mkdir(exist_ok=True)


# ─── Config & Session ────────────────────────────────────────────
def load_config():
    if CONFIG_FILE.exists():
        try:
            with open(CONFIG_FILE, "r", encoding="utf-8") as f:
                return json.load(f)
        except:
            pass
    return {"api_key": "", "model": DEFAULT_MODEL}


def save_config(config):
    with open(CONFIG_FILE, "w", encoding="utf-8") as f:
        json.dump(config, f, indent=2)


def load_session():
    if SESSION_FILE.exists():
        try:
            with open(SESSION_FILE, "r", encoding="utf-8") as f:
                return json.load(f)
        except:
            pass
    return {"messages": [], "created": time.time()}


def save_session(session):
    with open(SESSION_FILE, "w", encoding="utf-8") as f:
        json.dump(session, f, indent=2, ensure_ascii=False)


# ─── System Info Collection ──────────────────────────────────────
def collect_system_info():
    """Collect system information for diagnostic purposes."""
    info = {}
    
    # OS info
    info["os_name"] = platform.system()
    info["os_version"] = platform.version()
    info["os_release"] = platform.release()
    info["machine"] = platform.machine()
    info["processor"] = platform.processor() or "Unknown"
    info["hostname"] = socket.gethostname()
    
    # CPU
    try:
        info["cpu_count"] = psutil.cpu_count(logical=True)
        info["cpu_freq"] = round(psutil.cpu_freq().current, 1) if psutil.cpu_freq() else 0
    except:
        info["cpu_count"] = "N/A"
        info["cpu_freq"] = "N/A"
    
    # Memory
    try:
        ram = psutil.virtual_memory()
        info["ram_total_gb"] = round(ram.total / (1024**3), 1)
        info["ram_available_gb"] = round(ram.available / (1024**3), 1)
        info["ram_percent_used"] = ram.percent
    except:
        info["ram_total_gb"] = "N/A"
        info["ram_available_gb"] = "N/A"
        info["ram_percent_used"] = "N/A"
    
    # Disk usage
    disks = {}
    try:
        for partition in psutil.disk_partitions(all=False):
            try:
                usage = psutil.disk_usage(partition.mountpoint)
                label = partition.device.replace("\\\\?\\", "")
                disks[label] = {
                    "mount": partition.mountpoint,
                    "fstype": partition.fstype,
                    "total_gb": round(usage.total / (1024**3), 1),
                    "free_gb": round(usage.free / (1024**3), 1),
                    "percent_used": usage.percent,
                }
            except:
                pass
    except:
        pass
    info["disks"] = disks
    
    # Boot time
    try:
        boot = psutil.boot_time()
        info["uptime_hours"] = round((time.time() - boot) / 3600, 1)
        info["last_boot"] = time.strftime("%Y-%m-%d %H:%M", time.localtime(boot))
    except:
        info["uptime_hours"] = "N/A"
        info["last_boot"] = "N/A"
    
    # Process count
    try:
        info["running_processes"] = len(psutil.pids())
    except:
        info["running_processes"] = "N/A"
    
    return info


# Need psutil for system info
try:
    import psutil
except ImportError:
    psutil = None


def get_system_info_text():
    """Format system info as human-readable text."""
    if not psutil:
        return (
            f"OS: {platform.system()} {platform.version()}\n"
            f"Machine: {platform.machine()}\n"
            f"Hostname: {socket.gethostname()}"
        )
    
    info = collect_system_info()
    lines = [
        f"=== System Information ===",
        f"OS: {info['os_name']} {info['os_version']}",
        f"Machine: {info['machine']}",
        f"Hostname: {info['hostname']}",
        f"CPU: {info['cpu_count']} cores @ {info['cpu_freq']}GHz",
        f"RAM: {info['ram_total_gb']}GB ({info['ram_percent_used']}% used)",
        f"Uptime: {info['uptime_hours']} hours",
        f"Running processes: {info['running_processes']}",
        f"",
        f"=== Disks ===",
    ]
    
    for dev, d in info.get("disks", {}).items():
        lines.append(
            f"  {dev} ({d['mount']}): "
            f"{d['free_gb']}GB free / {d['total_gb']}GB ({d['percent_used']}% used) [{d['fstype']}]"
        )
    
    return "\n".join(lines)


# ─── API Communication ───────────────────────────────────────────

# ─── Tool Definitions ────────────────────────────────────────────
TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "tool",
            "description": "Execute a tool call (terminal, read_file, write_file).",
            "parameters": {
                "type": "object",
                "properties": {
                    "tool": {
                        "type": "string",
                        "description": "Tool name (terminal, read_file, write_file)."
                    },
                    "arguments": {
                        "type": "string",
                        "description": "Arguments for the tool, format: arg1\x00arg2"
                    }
                },
                "required": ["tool", "arguments"]
            }
        }
    }
]

def exec_terminal(command):
    """Execute a shell command and return output."""
    try:
        if sys.platform == "win32":
            cmd = ["cmd", "/C", command]
        else:
            cmd = ["bash", "-c", command]
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        output = result.stdout
        if result.returncode != 0:
            output += "\n[Exit code: " + str(result.returncode) + "]"
        if result.stderr:
            output += "\n" + result.stderr
        if len(output) > 50000:
            output = output[:50000] + "\n... (truncated)"
        return output
    except Exception as e:
        return "[Error: " + str(e) + "]"

def exec_read_file(path):
    """Read a file and return its contents."""
    try:
        if not os.path.isabs(path):
            exe_dir = str(Path(sys.executable).parent) if getattr(sys, 'frozen', False) else str(Path.cwd())
            path = os.path.join(exe_dir, path)
        with open(path, 'r', encoding='utf-8', errors='replace') as f:
            content = f.read()
        if len(content) > 50000:
            content = content[:50000] + "\n... (truncated)"
        return content
    except Exception as e:
        return "[Error reading file: " + str(e) + "]"

def exec_write_file(path, content):
    """Write content to a file."""
    try:
        if not os.path.isabs(path):
            exe_dir = str(Path(sys.executable).parent) if getattr(sys, 'frozen', False) else str(Path.cwd())
            path = os.path.join(exe_dir, path)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, 'w', encoding='utf-8') as f:
            f.write(content)
        return "[OK] File written successfully."
    except Exception as e:
        return "[Error writing file: " + str(e) + "]"

def call_tool(tool_name, args_str):
    """Dispatch tool call."""
    parts = args_str.split("\x00", 1)
    if len(parts) == 2:
        if tool_name == "write_file":
            return exec_write_file(parts[0], parts[1])
    if tool_name == "terminal":
        return exec_terminal(args_str)
    elif tool_name == "read_file":
        return exec_read_file(args_str)
    return "[Unknown tool: " + tool_name + "]"


def send_message(api_key, model, user_input, session, callback=None):
    """Send message to Hermes API and get response with tool calling."""
    messages = session.get("messages", [])
    messages.append({"role": "user", "content": user_input})
    
    system_prompt = (
        "You are Hermes Agent, a diagnostic AI assistant running from a portable USB drive. "
        "The user may be non-technical. Help them diagnose and fix computer problems. "
        "Be concise, practical, and guide them step by step. "
        "Use simple language. Always provide clear, actionable instructions. "
        "If the user provides system info, analyze it and give specific recommendations. "
        "You have access to tools: terminal (execute shell commands), read_file (read file contents), write_file (write file contents). "
        "When the user asks to run commands, check files, or modify files, use the tool interface."
    )
    
    payload = {
        "model": model,
        "messages": [{"role": "system", "content": system_prompt}] + messages[-30:],
        "temperature": 0.7,
        "max_tokens": 4096,
        "tools": TOOLS,
    }
    
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }
    
    try:
        resp = requests.post(
            f"{API_BASE}/chat/completions",
            json=payload,
            headers=headers,
            timeout=120
        )
        
        if resp.status_code == 200:
            data = resp.json()
            reply = data["choices"][0]["message"]["content"]
            
            # Check for tool calls
            tool_calls = data["choices"][0]["message"].get("tool_calls", [])
            if tool_calls:
                for tc in tool_calls:
                    func = tc.get("function", {})
                    tool_name = func.get("name", "")
                    args_str = func.get("arguments", "{}")
                    try:
                        import ast
                        args_dict = ast.literal_eval(args_str) if isinstance(args_str, str) else args_str
                        args_val = args_dict.get("arguments", args_str) if isinstance(args_dict, dict) else args_str
                    except:
                        args_val = args_str
                    
                    tool_result = call_tool(tool_name, args_val)
                    messages.append({"role": "assistant", "content": reply})
                    messages.append({"role": "tool", "content": tool_result, "tool_call_id": tc.get("id", "")})
                    
                    # Call API again with tool result
                    payload2 = {
                        "model": model,
                        "messages": [{"role": "system", "content": system_prompt}] + messages[-30:],
                        "temperature": 0.7,
                        "max_tokens": 4096,
                        "tools": TOOLS,
                    }
                    resp2 = requests.post(
                        f"{API_BASE}/chat/completions",
                        json=payload2,
                        headers=headers,
                        timeout=120
                    )
                    if resp2.status_code == 200:
                        data2 = resp2.json()
                        reply = data2["choices"][0]["message"]["content"]
                        messages.append({"role": "assistant", "content": reply})
            
            messages.append({"role": "assistant", "content": reply})
            session["messages"] = messages
            save_session(session)
            return reply
        elif resp.status_code == 401:
            return "❌ API Key无效，请检查密钥是否正确。"
        elif resp.status_code == 429:
            return "⏳ 请求太频繁，请稍等片刻再试。"
        else:
            return f"❌ 错误: HTTP {resp.status_code}"
            
    except requests.exceptions.Timeout:
        return "⏰ 请求超时，请检查网络连接。"
    except requests.exceptions.ConnectionError:
        return "🔌 无法连接服务器，请检查网络。"
    except Exception as e:
        return f"❌ 错误: {str(e)}"


# ─── GUI Application ─────────────────────────────────────────────
class HermesGUI:
    def __init__(self, root):
        self.root = root
        self.root.title("Hermes 诊断助手 v2.0")
        self.root.geometry("700x550")
        self.root.minsize(500, 400)
        
        # Load config
        config = load_config()
        self.api_key = config.get("api_key", "")
        self.model = config.get("model", DEFAULT_MODEL)
        self.session = load_session()
        
        # Setup UI
        self._setup_ui()
        
        # Welcome message
        self._add_message("system", "欢迎使用 Hermes 诊断助手！\n\n我是您的AI诊断助手，可以帮助您解决电脑问题。\n\n请描述您遇到的问题，我会一步步指导您修复。")
        
        # Auto-collect system info on first run
        if not self.session.get("messages"):
            self._auto_diagnose()
    
    def _setup_ui(self):
        """Build the GUI layout."""
        # Main container
        main_frame = ttk.Frame(self.root, padding=10)
        main_frame.pack(fill=tk.BOTH, expand=True)
        
        # Chat area
        chat_frame = ttk.Frame(main_frame)
        chat_frame.pack(fill=tk.BOTH, expand=True, pady=(0, 10))
        
        self.chat_display = scrolledtext.ScrolledText(
            chat_frame,
            wrap=tk.WORD,
            state=tk.DISABLED,
            font=("Microsoft YaHei UI", 10),
            bg="#1e1e1e",
            fg="#d4d4d4",
            relief=tk.FLAT,
            padx=10,
            pady=10,
        )
        self.chat_display.pack(fill=tk.BOTH, expand=True)
        
        # Tag colors for different message types
        self.chat_display.tag_config("user", foreground="#4ec9b0")
        self.chat_display.tag_config("hermes", foreground="#569cd6")
        self.chat_display.tag_config("system", foreground="#6a9955")
        self.chat_display.tag_config("error", foreground="#f44747")
        
        # Input area
        input_frame = ttk.Frame(main_frame)
        input_frame.pack(fill=tk.X)
        
        self.input_var = tk.StringVar()
        self.input_var.trace_add("write", self._on_input_change)
        
        self.input_field = ttk.Entry(
            input_frame,
            textvariable=self.input_var,
            font=("Microsoft YaHei UI", 10),
        )
        self.input_field.pack(side=tk.LEFT, fill=tk.X, expand=True, padx=(0, 5))
        self.input_field.bind("<Return>", lambda e: self._send_message())
        self.input_field.focus_set()
        
        # Buttons
        btn_frame = ttk.Frame(input_frame)
        btn_frame.pack(side=tk.RIGHT)
        
        self.send_btn = ttk.Button(
            btn_frame,
            text="发送 ▶",
            command=self._send_message,
            width=8,
        )
        self.send_btn.pack(side=tk.RIGHT, padx=(0, 5))
        
        ttk.Button(
            btn_frame,
            text="📋 系统信息",
            command=self._show_system_info,
            width=12,
        ).pack(side=tk.RIGHT, padx=(0, 5))
        
        ttk.Button(
            btn_frame,
            text="🔄 清空",
            command=self._clear_chat,
            width=8,
        ).pack(side=tk.RIGHT)
        
        # Status bar
        self.status_var = tk.StringVar(value="就绪")
        status_bar = ttk.Label(
            main_frame,
            textvariable=self.status_var,
            relief=tk.SUNKEN,
            anchor=tk.W,
        )
        status_bar.pack(fill=tk.X, pady=(5, 0))
    
    def _add_message(self, msg_type, text):
        """Add a message to the chat display."""
        self.chat_display.config(state=tk.NORMAL)
        
        if msg_type == "user":
            prefix = "您> "
            tag = "user"
        elif msg_type == "hermes":
            prefix = "Hermes> "
            tag = "hermes"
        elif msg_type == "error":
            prefix = "❌ "
            tag = "error"
        else:
            prefix = "[系统] "
            tag = "system"
        
        self.chat_display.insert(tk.END, prefix, tag)
        self.chat_display.insert(tk.END, text + "\n\n", tag)
        
        self.chat_display.config(state=tk.DISABLED)
        self.chat_display.see(tk.END)
    
    def _send_message(self):
        """Send user message to API."""
        user_input = self.input_var.get().strip()
        if not user_input:
            return
        
        # Clear input
        self.input_var.set("")
        
        # Add user message to chat
        self._add_message("user", user_input)
        
        # Disable input while thinking
        self.send_btn.config(state=tk.DISABLED)
        self.status_var.set("思考中...")
        
        # Send in background thread
        def api_thread():
            reply = send_message(self.api_key, self.model, user_input, self.session)
            self.root.after(0, lambda: self._on_reply(reply))
        
        threading.Thread(target=api_thread, daemon=True).start()
    
    def _on_reply(self, reply):
        """Handle API response."""
        self._add_message("hermes", reply)
        self.send_btn.config(state=tk.NORMAL)
        self.status_var.set("就绪")
        self.input_field.focus_set()
    
    def _auto_diagnose(self):
        """Auto-collect system info on first run."""
        if not psutil:
            return
        
        try:
            sys_info = get_system_info_text()
            self._add_message("system", f"正在收集系统信息...\n\n{sys_info}")
            
            # Send to Hermes for analysis
            def analyze_thread():
                prompt = (
                    "这是目标电脑的系统信息，请分析并给出初步诊断建议：\n\n" + sys_info
                )
                reply = send_message(self.api_key, self.model, prompt, self.session)
                self.root.after(0, lambda: self._add_message("hermes", reply))
            
            threading.Thread(target=analyze_thread, daemon=True).start()
        except Exception as e:
            self._add_message("error", f"收集系统信息失败: {e}")
    
    def _show_system_info(self):
        """Show system info and optionally send to Hermes."""
        if not psutil:
            messagebox.showinfo("系统信息", "psutil未安装，无法收集详细信息")
            return
        
        sys_info = get_system_info_text()
        
        # Copy to clipboard
        self.root.clipboard_clear()
        self.root.clipboard_append(sys_info)
        self.root.update()
        
        messagebox.showinfo(
            "系统信息",
            f"系统信息已复制到剪贴板！\n\n"
            f"RAM: {sys_info.split(chr(10))[4] if len(sys_info.split(chr(10))) > 4 else 'N/A'}\n"
            f"Uptime: {sys_info.split(chr(10))[-2] if len(sys_info.split(chr(10))) >= 2 else 'N/A'}\n\n"
            f"您可以将此信息发送给Hermes进行诊断。"
        )
        
        # Auto-send to Hermes
        self._add_message("system", "📋 系统信息已收集并自动发送给Hermes分析...")
        
        def analyze_thread():
            reply = send_message(
                self.api_key,
                self.model,
                f"请分析以下系统信息并给出建议：\n\n{sys_info}",
                self.session
            )
            self.root.after(0, lambda: self._add_message("hermes", reply))
        
        threading.Thread(target=analyze_thread, daemon=True).start()
    
    def _clear_chat(self):
        """Clear chat history."""
        self.session["messages"] = []
        save_session(self.session)
        self.chat_display.config(state=tk.NORMAL)
        self.chat_display.delete("1.0", tk.END)
        self.chat_display.config(state=tk.DISABLED)
        self._add_message("system", "对话已清空。请描述您的问题。")
    
    def _on_input_change(self, *args):
        """Enable/disable send button based on input."""
        if self.input_var.get().strip():
            self.send_btn.config(state=tk.NORMAL)
        else:
            self.send_btn.config(state=tk.DISABLED)


# ─── Entry Point ─────────────────────────────────────────────────
def main():
    # Check API key - use GUI dialog instead of input()
    config = load_config()
    if not config.get("api_key"):
        # Launch a simple Tk dialog for API key entry
        try:
            import tkinter as tk
            from tkinter import simpledialog
            
            # Create a hidden root window
            root = tk.Tk()
            root.withdraw()  # Hide main window
            
            # Prompt for API key
            api_key = simpledialog.askstring(
                "Hermes 诊断助手",
                "请输入您的 API Key:\n\n（首次运行需要输入API Key）",
                show="*"
            )
            root.destroy()
            
            if not api_key or not api_key.strip():
                # User cancelled or empty
                messagebox.showwarning("提示", "未输入API Key，程序将退出。")
                sys.exit(1)
            
            config["api_key"] = api_key.strip()
            save_config(config)
        except Exception as e:
            print(f"GUI对话框失败，尝试CLI模式: {e}")
            # Fallback to CLI mode
            api_key = input("请输入您的 API Key: ").strip()
            if not api_key:
                print("未输入API Key，退出。")
                sys.exit(1)
            config["api_key"] = api_key
            save_config(config)
    else:
        # API key exists, save it back in case we need to pass to cli_mode
        pass
    
    # Launch GUI
    try:
        root = tk.Tk()
        root.iconbitmap(default="")  # No icon on Linux
        app = HermesGUI(root)
        root.mainloop()
    except Exception as e:
        print(f"GUI启动失败: {e}")
        print("切换到命令行模式...")
        _cli_mode(config)


def _cli_mode(config):
    """Fallback CLI mode if GUI fails."""
    api_key = config["api_key"]
    model = config.get("model", DEFAULT_MODEL)
    session = load_session()
    
    print("\n" + "="*60)
    print("  HERMES DIAGNOSTIC CLIENT - CLI MODE")
    print("="*60)
    
    while True:
        try:
            user_input = input("\nYou> ").strip()
        except (EOFError, KeyboardInterrupt):
            print("\n再见！")
            break
        
        if not user_input:
            continue
        
        if user_input == "/quit":
            print("再见！")
            break
        elif user_input == "/info":
            if psutil:
                print(get_system_info_text())
            else:
                print("psutil未安装，无法收集系统信息。")
        else:
            reply = send_message(api_key, model, user_input, session)
            print(f"\nHermes> {reply}")


if __name__ == "__main__":
    main()

