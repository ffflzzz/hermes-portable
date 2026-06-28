#!/usr/bin/env python3
"""
Hermes Agent Lite - Portable Edition
A minimal Hermes Agent with terminal + file tools, packaged as a single exe.
Connects to the same API as full Hermes but runs standalone.

Features:
- Terminal tool: execute shell commands
- File tool: read/write/search/patch files
- Full Hermes conversation loop with tool calling
- Same model/provider as your main Hermes config
"""

import os
import sys
import json
import time
import subprocess
import platform
import socket
import traceback
import threading

# Explicit imports to prevent PyInstaller stripping
import requests
import tkinter  # noqa: F401

# ─── Configuration ───────────────────────────────────────────────────────
API_BASE = "https://apihub.agnes-ai.com/v1"
DEFAULT_MODEL = "agnes-2.0-flash"

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
DATA_DIR = os.path.join(SCRIPT_DIR, ".hermes_lite")
os.makedirs(DATA_DIR, exist_ok=True)

CONFIG_FILE = os.path.join(DATA_DIR, "config.json")
SESSION_DIR = os.path.join(DATA_DIR, "sessions")
os.makedirs(SESSION_DIR, exist_ok=True)

SYSTEM_PROMPT = """You are Hermes Agent Lite, a diagnostic AI assistant with full system access.

You have these tools available:
- terminal: Execute shell commands on the local system. Returns stdout/stderr.
- read_file: Read a file's contents.
- write_file: Write content to a file.
- search_files: Search for files by name or content.
- patch: Make targeted edits to files.

When the user asks a question:
1. Think step by step about what tools you need
2. Call tools as needed to gather information and solve the problem
3. Return clear, actionable results
4. For non-technical users, explain in simple terms

Always use the local system's shell (bash on Linux, PowerShell/cmd on Windows).
Be thorough but concise in your responses."""


# ─── Config Management ──────────────────────────────────────────────────
def load_config():
    if os.path.exists(CONFIG_FILE):
        try:
            with open(CONFIG_FILE, "r", encoding="utf-8") as f:
                return json.load(f)
        except:
            pass
    return {"api_key": "", "model": DEFAULT_MODEL}


def save_config(cfg):
    with open(CONFIG_FILE, "w", encoding="utf-8") as f:
        json.dump(cfg, f, indent=2)


# ─── Tool Implementations ───────────────────────────────────────────────
def tool_terminal(command, cwd=None, timeout=180):
    """Execute a shell command and return output."""
    try:
        result = subprocess.run(
            command,
            shell=True,
            capture_output=True,
            text=True,
            timeout=timeout,
            cwd=cwd or SCRIPT_DIR
        )
        output = result.stdout
        if result.stderr:
            output += "\n" + result.stderr
        output = output[:50000]  # Cap output size
        if result.returncode != 0:
            output += f"\n[Exit code: {result.returncode}]"
        return json.dumps({"success": True, "output": output, "returncode": result.returncode})
    except subprocess.TimeoutExpired:
        return json.dumps({"success": False, "error": "Command timed out"})
    except Exception as e:
        return json.dumps({"success": False, "error": str(e)})


def tool_read_file(path):
    """Read file contents."""
    try:
        # Resolve relative to SCRIPT_DIR if not absolute
        if not os.path.isabs(path):
            path = os.path.join(SCRIPT_DIR, path)
        with open(path, "r", encoding="utf-8") as f:
            content = f.read()
        content = content[:50000]
        return json.dumps({"success": True, "content": content, "lines": len(content.splitlines())})
    except Exception as e:
        return json.dumps({"success": False, "error": str(e)})


def tool_write_file(path, content):
    """Write content to file."""
    try:
        if not os.path.isabs(path):
            path = os.path.join(SCRIPT_DIR, path)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w", encoding="utf-8") as f:
            f.write(content)
        return json.dumps({"success": True, "bytes_written": len(content)})
    except Exception as e:
        return json.dumps({"success": False, "error": str(e)})


def tool_search_files(pattern, path=None, target="content"):
    """Search for files or content."""
    try:
        search_path = path or SCRIPT_DIR
        if target == "files":
            import fnmatch
            matches = []
            for root, dirs, files in os.walk(search_path):
                for f in files:
                    if fnmatch.fnmatch(f, pattern):
                        matches.append(os.path.join(root, f))
        else:
            # Content search using subprocess grep
            import subprocess
            cmd = f'grep -rli "{pattern}" "{search_path}" 2>/dev/null | head -50'
            result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=30)
            matches = [m.strip() for m in result.stdout.strip().split('\n') if m.strip()]
        return json.dumps({"success": True, "matches": matches, "count": len(matches)})
    except Exception as e:
        return json.dumps({"success": False, "error": str(e)})


def tool_patch_file(path, old_string, new_string, replace_all=False):
    """Make targeted edits to a file."""
    try:
        if not os.path.isabs(path):
            path = os.path.join(SCRIPT_DIR, path)
        with open(path, "r", encoding="utf-8") as f:
            content = f.read()
        
        if replace_all:
            new_content = content.replace(old_string, new_string)
        else:
            if old_string not in content:
                return json.dumps({"success": False, "error": "String not found"})
            new_content = content.replace(old_string, new_string, 1)
        
        with open(path, "w", encoding="utf-8") as f:
            f.write(new_content)
        
        return json.dumps({"success": True, "replacements": 1 if not replace_all else content.count(old_string)})
    except Exception as e:
        return json.dumps({"success": False, "error": str(e)})


# Tool registry for the LLM
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
                    },
                    "cwd": {
                        "type": "string",
                        "description": "Working directory (optional)"
                    },
                    "timeout": {
                        "type": "integer",
                        "description": "Timeout in seconds (default 180)"
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
            "description": "Write content to a file, creating it if it doesn't exist.",
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
    },
    {
        "type": "function",
        "function": {
            "name": "search_files",
            "description": "Search for files by name pattern or content.",
            "parameters": {
                "type": "object",
                "properties": {
                    "pattern": {
                        "type": "string",
                        "description": "Search pattern (glob for files, regex for content)"
                    },
                    "path": {
                        "type": "string",
                        "description": "Directory to search in"
                    },
                    "target": {
                        "type": "string",
                        "enum": ["files", "content"],
                        "description": "Search in filenames or file contents"
                    }
                },
                "required": ["pattern"]
            }
        }
    },
    {
        "type": "function",
        "function": {
            "name": "patch_file",
            "description": "Make targeted edits to a file.",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {
                        "type": "string",
                        "description": "File path to edit"
                    },
                    "old_string": {
                        "type": "string",
                        "description": "Text to find"
                    },
                    "new_string": {
                        "type": "string",
                        "description": "Replacement text"
                    },
                    "replace_all": {
                        "type": "boolean",
                        "description": "Replace all occurrences"
                    }
                },
                "required": ["path", "old_string", "new_string"]
            }
        }
    }
]

# Tool handler mapping - use a class to prevent PyInstaller stripping
class ToolRegistry:
    terminal = tool_terminal
    read_file = tool_read_file
    write_file = tool_write_file
    search_files = tool_search_files
    patch_file = tool_patch_file
    
    @classmethod
    def get(cls, name):
        return getattr(cls, name, None)
    
    @classmethod
    def names(cls):
        return ['terminal', 'read_file', 'write_file', 'search_files', 'patch_file']

TOOL_HANDLERS = ToolRegistry()


# ─── Session Management ─────────────────────────────────────────────────
def get_session_path():
    return os.path.join(SESSION_DIR, "default.json")


def load_session():
    p = get_session_path()
    if os.path.exists(p):
        try:
            with open(p, "r", encoding="utf-8") as f:
                return json.load(f)
        except:
            pass
    return {"messages": [], "created": time.time()}


def save_session(sess):
    with open(get_session_path(), "w", encoding="utf-8") as f:
        json.dump(sess, f, indent=2, ensure_ascii=False)


# ─── API Call ────────────────────────────────────────────────────────────
def call_api(messages, api_key, model, tools=None, max_turns=10):
    """Call the LLM API with tool calling support."""
    system_msg = {"role": "system", "content": SYSTEM_PROMPT}
    all_messages = [system_msg] + messages
    
    for turn in range(max_turns):
        payload = {
            "model": model,
            "messages": all_messages,
            "temperature": 0.7,
            "max_tokens": 8192,
            "top_p": 0.9,
        }
        if tools:
            payload["tools"] = tools
        
        try:
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
                
                else:
                    # Final text response
                    reply = choice["message"].get("content", "")
                    all_messages.append({"role": "assistant", "content": reply})
                    return all_messages, reply
                    
            elif r.status_code == 401:
                return all_messages, "❌ API Key无效"
            elif r.status_code == 429:
                return all_messages, "⏳ 请求太频繁，请稍后再试"
            else:
                return all_messages, f"❌ HTTP {r.status_code}: {r.text[:200]}"
                
        except requests.exceptions.Timeout:
            return all_messages, "⏰ 请求超时"
        except requests.exceptions.ConnectionError:
            return all_messages, "🔌 无法连接服务器"
        except Exception as e:
            return all_messages, f"❌ {e}"
    
    return all_messages, "⚠️ 达到最大轮次限制"


# ─── CLI Mode ───────────────────────────────────────────────────────────
def banner():
    print()
    print("=" * 60)
    print("  Hermes Agent Lite v1.0 - 精简版诊断助手")
    print("=" * 60)
    print()
    print("  功能: terminal | read_file | write_file | search_files | patch_file")
    print()
    print("  命令:")
    print("    /info     - 显示系统信息")
    print("    /clear    - 清空对话")
    print("    /reset    - 重置API Key")
    print("    /quit     - 退出")
    print()
    print("  直接输入问题即可开始对话")
    print()


def collect_sys_info():
    info = []
    info.append(f"OS: {platform.system()} {platform.version()}")
    info.append(f"Machine: {platform.machine()}")
    info.append(f"Hostname: {socket.gethostname()}")
    info.append(f"CPU Cores: {os.cpu_count()}")
    info.append(f"Working Dir: {SCRIPT_DIR}")
    return "\n".join(info)


def cli_mode(api_key, model):
    banner()
    session = load_session()
    
    # Auto system info on first run
    if not session.get("messages"):
        print("📋 正在收集系统信息...")
        sys_info = collect_sys_info()
        print(sys_info)
        print("\n正在发送给Hermes分析...\n")
        session["messages"], reply = call_api(
            [{"role": "user", "content": f"这是本机系统信息，请分析并给出初步诊断建议：\n{sys_info}"}],
            api_key, model, TOOLS
        )
        session["messages"][-1]["role"] = "assistant"  # Fix role
        save_session(session)
        print(f"Hermes> {reply}\n")
    
    while True:
        try:
            user_input = input("你> ").strip()
        except (EOFError, KeyboardInterrupt):
            print("\n再见！")
            break
        
        if not user_input:
            continue
        
        if user_input in ("/quit", "/exit"):
            print("再见！")
            break
        elif user_input == "/clear":
            session["messages"] = []
            save_session(session)
            print("对话已清空。\n")
        elif user_input == "/reset":
            save_config({"api_key": "", "model": DEFAULT_MODEL})
            print("API Key已清除。重启程序重新输入。")
            break
        elif user_input == "/info":
            print(collect_sys_info())
            print()
        else:
            print("思考中...", end="", flush=True)
            session["messages"], reply = call_api(
                session.get("messages", []), api_key, model, TOOLS
            )
            print("\b" * 12 + "     \b" * 5)
            print(f"\nHermes> {reply}\n")


# ─── Entry Point ────────────────────────────────────────────────────────
def main():
    cfg = load_config()
    
    if not cfg.get("api_key"):
        print("首次运行，请输入你的 API Key：")
        api_key = input("> ").strip()
        if not api_key:
            print("未输入API Key，退出。")
            sys.exit(1)
        cfg["api_key"] = api_key
        save_config(cfg)
        print("API Key已保存。\n")
    
    api_key = cfg["api_key"]
    model = cfg.get("model", DEFAULT_MODEL)
    
    # Always run CLI mode (no GUI needed for this lite version)
    cli_mode(api_key, model)


if __name__ == "__main__":
    import requests
    
    # Static references to prevent PyInstaller from stripping tool functions
    # These are never called at runtime but ensure PyInstaller tracks them
    _tool_funcs = [
        tool_terminal,
        tool_read_file,
        tool_write_file,
        tool_search_files,
        tool_patch_file,
    ]
    _tool_handlers_ref = TOOL_HANDLERS
    _tools_schema_ref = TOOLS
    
    main()
