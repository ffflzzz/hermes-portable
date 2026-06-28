#!/usr/bin/env python3
"""
Hermes Diagnostic Client - Portable Edition
A lightweight chat client for Hermes Agent API.
No installation needed. Just double-click and enter your API key.
"""

import os
import sys
import json
import hashlib
import time
import threading
import webbrowser
from pathlib import Path

try:
    import requests
except ImportError:
    print("ERROR: requests library not found.")
    print("Please install it: python -m pip install requests")
    input("Press Enter to exit...")
    sys.exit(1)

# ─── Configuration ───────────────────────────────────────────────
API_BASE = "https://apihub.agnes-ai.com/v1"
DEFAULT_MODEL = "agnes-2.0-flash"

# Data directory: same folder as this script, or user's home
SCRIPT_DIR = Path(__file__).parent.resolve()
DATA_DIR = SCRIPT_DIR / ".hermes_portable"
DATA_DIR.mkdir(exist_ok=True)

CONFIG_FILE = DATA_DIR / "config.json"
SESSION_FILE = DATA_DIR / "sessions"
SESSION_FILE.mkdir(exist_ok=True)


def load_config():
    """Load saved API key and settings."""
    if CONFIG_FILE.exists():
        try:
            with open(CONFIG_FILE, "r", encoding="utf-8") as f:
                return json.load(f)
        except:
            pass
    return {"api_key": "", "model": DEFAULT_MODEL}


def save_config(config):
    """Save API key and settings."""
    with open(CONFIG_FILE, "w", encoding="utf-8") as f:
        json.dump(config, f, indent=2)


def get_api_key():
    """Prompt user for API key if not saved."""
    config = load_config()
    if config.get("api_key"):
        print(f"\nUsing saved API key: {config['api_key'][:8]}...{config['api_key'][-4:]}")
        return config["api_key"], config.get("model", DEFAULT_MODEL)
    
    print("\n" + "="*60)
    print("  HERMES DIAGNOSTIC CLIENT - PORTABLE EDITION")
    print("="*60)
    print("\nWelcome! This is a portable diagnostic tool for Hermes Agent.")
    print("You need an API key to use it.\n")
    
    # Try clipboard
    try:
        import subprocess
        result = subprocess.run(["clip", "/O"], capture_output=True, timeout=2)
        if result.returncode != 0:
            result = subprocess.run(["pbpaste"], capture_output=True, timeout=2)
        if result.returncode == 0 and result.stdout.strip():
            clipboard = result.stdout.decode("utf-8", errors="ignore").strip()
            if len(clipboard) > 10:
                print(f"Found API key in clipboard: {clipboard[:8]}...")
                paste_confirm = input("Paste it? (Y/n): ").strip().lower()
                if paste_confirm != "n":
                    api_key = clipboard
                    model = DEFAULT_MODEL
                    save_config({"api_key": api_key, "model": model})
                    print("API key saved. Let's go!\n")
                    return api_key, model
    except:
        pass
    
    api_key = input("Enter your API key: ").strip()
    if not api_key:
        print("No API key provided. Exiting.")
        sys.exit(1)
    
    model = input(f"Model [{DEFAULT_MODEL}]: ").strip() or DEFAULT_MODEL
    
    save_config({"api_key": api_key, "model": model})
    print("API key saved. Let's go!\n")
    return api_key, model


def get_session_file(user_id="default"):
    """Get session file path for this user."""
    return SESSION_FILE / f"{user_id}.json"


def load_session(user_id="default"):
    """Load conversation history."""
    sf = get_session_file(user_id)
    if sf.exists():
        try:
            with open(sf, "r", encoding="utf-8") as f:
                return json.load(f)
        except:
            pass
    return {"messages": [], "created": time.time()}


def save_session(session):
    """Save conversation history."""
    sf = get_session_file()
    with open(sf, "w", encoding="utf-8") as f:
        json.dump(session, f, indent=2, ensure_ascii=False)


def send_message(api_key, model, user_input, session):
    """Send a message to the Hermes API and get response."""
    # Build messages list
    messages = session.get("messages", [])
    messages.append({"role": "user", "content": user_input})
    
    # System prompt - diagnostic mode
    system_msg = {
        "role": "system", 
        "content": (
            "You are Hermes Agent, a diagnostic AI assistant. "
            "The user has plugged in a portable diagnostic USB drive. "
            "Help them diagnose and fix computer problems. "
            "Be concise, practical, and guide them step by step. "
            "Use simple language that non-technical users can understand. "
            "Always provide clear, actionable instructions."
        )
    }
    
    payload = {
        "model": model,
        "messages": [system_msg] + messages[-20:],  # Keep last 20 messages
        "temperature": 0.7,
        "max_tokens": 4096,
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
            messages.append({"role": "assistant", "content": reply})
            session["messages"] = messages
            save_session(session)
            return reply
        elif resp.status_code == 401:
            return "❌ API Key无效，请重新运行程序输入正确的密钥。"
        elif resp.status_code == 429:
            return "⏳ 请求太频繁，请稍等片刻再试。"
        else:
            return f"❌ 错误: HTTP {resp.status_code}\n{resp.text[:200]}"
            
    except requests.exceptions.Timeout:
        return "⏰ 请求超时，请检查网络连接。"
    except requests.exceptions.ConnectionError:
        return "🔌 无法连接到服务器，请检查网络连接。"
    except Exception as e:
        return f"❌ 错误: {str(e)}"


def print_banner():
    """Print welcome banner."""
    print("\n" + "="*60)
    print("  HERMES DIAGNOSTIC CLIENT v1.0")
    print("  Portable Edition - Plug & Play")
    print("="*60)
    print("\nCommands:")
    print("  /clear    - Clear conversation")
    print("  /reset    - Reset API key")
    print("  /quit     - Exit")
    print("  /help     - Show this help")
    print("\nJust type your question and press Enter.\n")


def main():
    """Main entry point."""
    api_key, model = get_api_key()
    session = load_session()
    print_banner()
    
    # Check for system info on first run
    if not session.get("messages"):
        print("📋 Collecting system information...\n")
        try:
            import platform
            import socket
            hostname = socket.gethostname()
            os_name = platform.system()
            os_ver = platform.version()
            print(f"Host: {hostname}")
            print(f"OS: {os_name} {os_ver}")
            print(f"Python: {platform.python_version()}")
            print(f"Location: {SCRIPT_DIR}")
        except:
            pass
        print("\nSystem info collected. Ready to diagnose!\n")
    
    while True:
        try:
            user_input = input("You> ").strip()
        except (EOFError, KeyboardInterrupt):
            print("\nGoodbye!")
            break
        
        if not user_input:
            continue
        
        # Handle commands
        if user_input.startswith("/"):
            cmd = user_input.lower()
            if cmd == "/clear":
                session["messages"] = []
                save_session(session)
                print("Conversation cleared.\n")
            elif cmd == "/reset":
                save_config({"api_key": "", "model": DEFAULT_MODEL})
                print("API key cleared. Restart the program to enter a new key.")
                break
            elif cmd == "/quit" or cmd == "/exit":
                print("Goodbye!")
                break
            elif cmd == "/help":
                print_banner()
            elif cmd == "/info":
                try:
                    import platform, socket
                    print(f"Hostname: {socket.gethostname()}")
                    print(f"OS: {platform.system()} {platform.version()}")
                    print(f"Python: {platform.python_version()}")
                    print(f"CWD: {SCRIPT_DIR}")
                except Exception as e:
                    print(f"Error: {e}")
            else:
                print(f"Unknown command: {cmd}. Type /help for commands.")
            continue
        
        # Send to API
        print("Thinking...", end="", flush=True)
        reply = send_message(api_key, model, user_input, session)
        print("\b\b\b     \b\b\b")
        print(f"\nHermes> {reply}\n")


if __name__ == "__main__":
    main()
