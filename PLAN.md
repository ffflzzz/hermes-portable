# Hermes便携版 - 三合一增强规划

## 目标
制作一个真正"即插即用"的诊断U盘，小白双击就能用，无需任何前置条件。

---

## Phase 1: 便携Python打包 (Portable Python Bundle)

### 问题
当前方案要求目标电脑预装Python 3.10+，但小白电脑很可能没有。

### 方案
将Python运行时打包进U盘，实现真正的免安装。

### 技术选型
- **工具**: Nuitka 或 PyInstaller + Portable Python
- **推荐**: 使用 `python-portable` 项目 或自制便携包
- **备选**: 用Nuitka把client.py编译成单文件exe（自带Python运行时）

### 具体步骤
1. 在C630上安装Nuitka: `pip install nuitka`
2. 编译client.py为单文件exe:
   ```bash
   nuitka --standalone --onefile --windows-icon-from-ico=icon.ico --enable-plugin=pyqt5 client.py
   ```
3. 测试生成的exe在其他Windows机器上能否运行
4. 体积目标: ≤50MB（含requests依赖）

### 验证标准
- 在空白Windows虚拟机（无Python）中双击exe能运行
- 首次运行自动下载requests（如缺失）
- 体积≤50MB

---

## Phase 2: 图形界面 (GUI with tkinter)

### 问题
当前命令行界面（CLI）对小白不够友好，看不懂提示。

### 方案
用tkinter做一个简洁的GUI窗口，类似QQ聊天界面。

### 设计
```
┌─────────────────────────────────────────────────┐
│  🤖 Hermes诊断助手                    [_][□][×] │
├─────────────────────────────────────────────────┤
│                                                  │
│  [系统] 欢迎使用Hermes诊断助手                    │
│         请输入您的问题...                         │
│                                                  │
│  [您]     电脑开不了机怎么办？                     │
│                                                  │
│  [Hermes] 请问是完全黑屏还是有错误代码？           │
│            1. 按下电源键后无任何反应               │
│            2. 有声音但屏幕黑屏                     │
│            3. 显示错误代码                         │
│                                                  │
├─────────────────────────────────────────────────┤
│  [___________________________][发送 ▶]           │
└─────────────────────────────────────────────────┘
```

### 具体步骤
1. 修改client.py，增加GUI模式
2. 使用tkinter（Python内置，无需额外安装）
3. 实现聊天界面：
   - 消息气泡（左边用户，右边Hermes）
   - 自动滚动
   - 快捷键Ctrl+Enter发送
4. 首次启动弹窗输入API Key
5. API Key保存到隐藏文件
6. 添加"系统信息"按钮（点击自动收集）

### 验证标准
- 在Windows上双击exe弹出GUI窗口
- 输入问题后能收到回复
- 界面简洁，小白看得懂
- 不需要命令行知识

---

## Phase 3: 自动系统诊断 (Auto-Diagnosis)

### 问题
小白不知道怎么说问题，需要自动收集信息辅助诊断。

### 方案
程序启动时自动收集目标电脑的系统信息，发送给Hermes进行分析。

### 收集的信息
**Windows:**
- 操作系统版本（Win10/Win11/Server）
- 硬件配置（CPU/内存/磁盘）
- 最近的事件日志（错误/警告）
- 开机时间（判断是否频繁重启）
- 磁盘空间（C盘是否满了）
- 已安装的安全软件

**Linux/Mac:**
- 发行版/版本
- 内核版本
- 磁盘使用情况
- 最近系统日志

### 具体步骤
1. 在client.py中增加`collect_system_info()`函数
2. 使用platform、psutil、subprocess等标准库
3. 首次运行时自动收集（不弹窗）
4. 将信息格式化后发送给Hermes
5. Hermes根据信息给出针对性建议

### 代码示例
```python
def collect_windows_info():
    import platform
    import psutil
    import subprocess
    
    info = {
        "os": platform.system(),
        "version": platform.version(),
        "machine": platform.machine(),
        "processor": platform.processor(),
        "ram_gb": round(psutil.virtual_memory().total / (1024**3), 1),
        "cpu_cores": psutil.cpu_count(),
    }
    
    # Get disk usage
    for drive in "ABCDEFGHIJKLMNOPQRSTUVWXYZ":
        path = f"{drive}:\\\"
        if os.path.exists(path):
            usage = psutil.disk_usage(path)
            info[f"{drive}_disk"] = {
                "total_gb": round(usage.total / (1024**3), 1),
                "free_gb": round(usage.free / (1024**3), 1),
                "percent_used": usage.percent,
            }
    
    # Get last boot time
    try:
        boot_time = psutil.boot_time()
        info["uptime_hours"] = round((time.time() - boot_time) / 3600, 1)
    except:
        pass
    
    return info
```

### 验证标准
- 自动收集信息不弹窗、不卡顿
- 信息准确（与实际系统一致）
- Hermes能根据信息给出有用建议
- 隐私保护（不收集个人文件内容）

---

## 最终交付物

### U盘结构
```
U盘/
├── hermes.exe          ← 单文件exe（便携Python+GUI+诊断）
├── README.txt          ← 使用说明（大字版）
└── icon.ico            ← 程序图标
```

### 用户操作流程
1. 插上U盘 → 打开"我的电脑"
2. 双击 `hermes.exe`
3. （首次）弹窗输入API Key → 保存
4. 自动弹出GUI窗口，显示系统信息
5. 在输入框打字描述问题 → 发送
6. 收到Hermes的诊断和建议
7. 继续对话，逐步解决问题

### 技术栈
- **语言**: Python 3.10+（便携版）
- **打包**: Nuitka → 单文件exe
- **GUI**: tkinter（内置，无需安装）
- **网络**: requests（内置或自动下载）
- **系统信息**: platform + psutil + subprocess

### 体积目标
- 总大小: ≤80MB
- 其中Python运行时: ≤50MB
- 其中程序代码: ≤1MB

### 时间估算
- Phase 1 (便携Python): 2-3小时
- Phase 2 (GUI): 3-4小时
- Phase 3 (自动诊断): 2-3小时
- 测试和优化: 2小时
- **总计: 约10小时**

---

## 风险与应对

| 风险 | 影响 | 应对 |
|------|------|------|
| Nuitka编译失败 | 无法生成exe | 备用PyInstaller |
| 杀毒误报 | 用户不敢运行 | 代码签名 + 白名单指南 |
| GUI兼容性 | 旧Windows显示异常 | 使用系统原生字体 |
| 网络问题 | 无法调API | 离线模式提示 |
| 体积过大 | U盘放不下 | 压缩Python运行时 |

---

## 后续迭代

1. **v1.1**: 支持多API Key切换
2. **v1.2**: 支持语音输入（麦克风）
3. **v1.3**: 支持截图诊断（自动截图发给Hermes）
4. **v2.0**: 集成远程协助（TeamViewer式）
