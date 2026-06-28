#!/usr/bin/env python3
"""
Hermes Diagnostic Client - CLI Mode
Pure terminal chat, no GUI needed.
"""

import os
import sys
import json
import time
import platform
import socket

try:
    import requests
except ImportError:
    print("[!] 缺少requests库，正在安装...")
    os.system(f'{sys.executable} -m pip install requests --quiet')
    import requests

# ─── Paths ───────────────────────────────────────────────────────
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
DATA_DIR = os.path.join(SCRIPT_DIR, ".hermes_portable")
os.makedirs(DATA_DIR, exist_ok=True)

CONFIG_FILE = os.path.join(DATA_DIR, "config.json")
SESSION_DIR = os.path.join(DATA_DIR, "sessions")
os.makedirs(SESSION_DIR, exist_ok=True)

API_BASE = "https://apihub.agnes-ai.com/v1"
DEFAULT_MODEL = "agnes-2.0-flash"


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


def collect_sys_info():
    info = []
    info.append(f"OS: {platform.system()} {platform.version()}")
    info.append(f"Machine: {platform.machine()}")
    info.append(f"Hostname: {socket.gethostname()}")
    try:
        import psutil
        info.append(f"CPU: {psutil.cpu_count()} cores")
        ram = psutil.virtual_memory()
        info.append(f"RAM: {round(ram.total/1024**3, 1)}GB ({ram.percent}% used)")
        try:
            info.append(f"Uptime: {round((time.time()-psutil.boot_time())/3600, 1)} hours")
        except:
            pass
        for part in psutil.disk_partitions():
            try:
                u = psutil.disk_usage(part.mountpoint)
                info.append(f"Disk {part.device}: {round(u.free/1024**3,1)}GB free / {round(u.total/1024**3,1)}GB ({u.percent}% used)")
            except:
                pass
    except ImportError:
        info.append("(psutil未安装，跳过详细系统信息)")
    return "\n".join(info)


def send_msg(api_key, model, user_input, session):
    msgs = session.get("messages", [])
    msgs.append({"role": "user", "content": user_input})

    system = (
        "You are Hermes Agent, a diagnostic AI assistant. "
        "Help users diagnose and fix computer problems step by step. "
        "Use simple language for non-technical users. "
        "Provide clear, actionable instructions."
    )

    payload = {
        "model": model,
        "messages": [{"role": "system", "content": system}] + msgs[-30:],
        "temperature": 0.7,
        "max_tokens": 4096,
    }

    try:
        r = requests.post(
            f"{API_BASE}/chat/completions",
            json=payload,
            headers={"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"},
            timeout=120
        )
        if r.status_code == 200:
            reply = r.json()["choices"][0]["message"]["content"]
            msgs.append({"role": "assistant", "content": reply})
            session["messages"] = msgs
            save_session(session)
            return reply
        elif r.status_code == 401:
            return "❌ API Key无效"
        elif r.status_code == 429:
            return "⏳ 请求太频繁，请稍后再试"
        else:
            return f"❌ HTTP {r.status_code}"
    except requests.exceptions.Timeout:
        return "⏰ 请求超时"
    except requests.exceptions.ConnectionError:
        return "🔌 无法连接服务器"
    except Exception as e:
        return f"❌ {e}"


def banner():
    print()
    print("=" * 50)
    print("  Hermes 诊断助手 v2.0 - 命令行模式")
    print("=" * 50)
    print()
    print("  命令:")
    print("    /info     - 显示系统信息")
    print("    /clear    - 清空对话")
    print("    /reset    - 重置API Key")
    print("    /quit     - 退出")
    print()
    print("  直接输入问题即可开始对话")
    print()


def main():
    cfg = load_config()

    # First run: get API key
    if not cfg.get("api_key"):
        print("首次运行，请输入你的 API Key：")
        print("(如果你还没有API Key，请联系购买渠道获取)")
        print()
        api_key = input("> ").strip()
        if not api_key:
            print("未输入API Key，退出。")
            sys.exit(1)
        cfg["api_key"] = api_key
        save_config(cfg)
        print("API Key已保存。\n")

    api_key = cfg["api_key"]
    model = cfg.get("model", DEFAULT_MODEL)
    session = load_session()

    banner()

    # Auto system info on first run
    if not session.get("messages"):
        print("📋 正在收集系统信息...")
        sys_info = collect_sys_info()
        print(sys_info)
        print("\n正在发送给Hermes分析...\n")
        reply = send_msg(api_key, model, f"这是本机系统信息，请分析并给出初步诊断建议：\n{sys_info}", session)
        print(f"Hermes> {reply}\n")

    while True:
        try:
            user_input = input("你> ").strip()
        except (EOFError, KeyboardInterrupt):
            print("\n再见！")
            break

        if not user_input:
            continue

        if user_input == "/quit" or user_input == "/exit":
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
            reply = send_msg(api_key, model, user_input, session)
            print("\b" * 12 + "     \b" * 5)
            print(f"\nHermes> {reply}\n")


if __name__ == "__main__":
    main()
