package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/lxn/walk"
)

var (
	configPath string
	AppDir     = ".hermes_lite"
)

func initPaths() {
	exePath, _ := os.Executable()
	d := filepath.Join(filepath.Dir(exePath), AppDir)
	os.MkdirAll(d, 0755)
	configPath = filepath.Join(d, "config.json")
}

type Config struct {
	APIKey string `json:"api_key"`
}

func loadConfig() Config {
	var cfg Config
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &cfg)
	}
	return cfg
}

func saveConfig(cfg Config) {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configPath, data, 0644)
}

type ChatMsg struct {
	Role    string
	Content string
	Meta    map[string]string
}

func execTerminal(command string) string {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("bash", "-c", command)
	}
	output, err := cmd.CombinedOutput()
	result := string(output)
	if err != nil {
		result += fmt.Sprintf("\n[Exit code: %v]", err)
	}
	if len(result) > 50000 {
		result = result[:50000] + "\n... (truncated)"
	}
	return result
}

func execReadFile(path string) string {
	if !filepath.IsAbs(path) {
		exePath, _ := os.Executable()
		path = filepath.Join(filepath.Dir(exePath), path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("[Error reading file: %v]", err)
	}
	content := string(data)
	if len(content) > 50000 {
		content = content[:50000] + "\n... (truncated)"
	}
	return content
}

func execWriteFile(path, content string) string {
	if !filepath.IsAbs(path) {
		exePath, _ := os.Executable()
		path = filepath.Join(filepath.Dir(exePath), path)
	}
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Sprintf("[Error writing file: %v]", err)
	}
	return "[OK] File written successfully."
}

func callAPI(messages []ChatMsg, apiKey string, toolHandler func(string, string) string) (string, []ChatMsg) {
	payload := map[string]interface{}{
		"model":    "mimo-v2.5-pro",
		"messages": messages,
		"stream":   false,
	}
	if toolHandler != nil {
		payload["tools"] = []map[string]interface{}{
			{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "tool",
					"description": "Execute a tool call.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"tool": map[string]interface{}{
								"type":        "string",
								"description": "Tool name (terminal, read_file, write_file).",
							},
							"arguments": map[string]interface{}{
								"type":        "string",
								"description": "Arguments for the tool.",
							},
						},
						"required": []string{"tool", "arguments"},
					},
				},
			},
		}
	}

	data, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://mimo-v2.5-pro.tp-cook-9d1f9e3a.kb.openai-proxy.com/v1/chat/completions", bytes.NewBuffer(data))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf("[API Error: %v]", err), messages
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("[API Error: %s]", body)
		messages = append(messages, ChatMsg{Role: "assistant", Content: msg})
		return msg, messages
	}

	choice := result["choices"].([]interface{})[0].(map[string]interface{})
	message := choice["message"].(map[string]interface{})
	content := ""
	if v, ok := message["content"]; ok {
		content = v.(string)
	}
	messages = append(messages, ChatMsg{Role: "assistant", Content: content})

	toolCalls, hasToolCalls := message["tool_calls"].([]interface{})
	if !hasToolCalls {
		return content, messages
	}

	for _, tc := range toolCalls {
		call := tc.(map[string]interface{})
		name := call["name"].(string)
		argsStr := call["arguments"].(string)
		var argsMap map[string]interface{}
		json.Unmarshal([]byte(argsStr), &argsMap)
		arg := ""
		if v, ok := argsMap["arguments"].(string); ok {
			arg = v
		}
		result := toolHandler(name, arg)
		messages = append(messages, ChatMsg{Role: "tool", Content: result, Meta: map[string]string{"tool": name}})
	}

	return callAPI(messages, apiKey, toolHandler)
}

type App struct {
	apiKey       string
	busy         bool
	mu           sync.Mutex
	messages     []ChatMsg
	mw           *walk.MainWindow
	statusLabel  *walk.Label
	sendBtn      *walk.PushButton
	chatEdit     *walk.TextEdit
	inputEntry   *walk.LineEdit
}

func (a *App) appendChat(role, content string) {
	prefix := ""
	switch role {
	case "user":
		prefix = fmt.Sprintf("[%s] 👤 你: ", time.Now().Format("15:04:05"))
	case "assistant":
		prefix = fmt.Sprintf("[%s] 🤖 Agent: ", time.Now().Format("15:04:05"))
	case "tool":
		prefix = fmt.Sprintf("[%s] 🔧 工具: ", time.Now().Format("15:04:05"))
	default:
		prefix = ""
	}
	line := prefix + strings.ReplaceAll(content, "\n", "\n  ") + "\n\n"
	a.chatEdit.AppendText(line)
}

func (a *App) send() {
	if a.busy {
		return
	}
	input := strings.TrimSpace(a.inputEntry.Text())
	if input == "" {
		return
	}

	a.appendChat("user", input)
	a.busy = true
	a.sendBtn.SetEnabled(false)
	a.statusLabel.SetText("⏳ 思考中...")

	go func() {
		a.mu.Lock()
		a.messages = append(a.messages, ChatMsg{Role: "user", Content: input})
		msgsCopy := make([]ChatMsg, len(a.messages))
		copy(msgsCopy, a.messages)
		a.mu.Unlock()

		reply, newMsgs := callAPI(msgsCopy, a.apiKey, func(toolName, arg string) string {
			parts := strings.SplitN(arg, "\x00", 2)
			if len(parts) == 2 {
				if toolName == "write_file" {
					return execWriteFile(parts[0], parts[1])
				}
			}
			switch toolName {
			case "terminal":
				return execTerminal(arg)
			case "read_file":
				return execReadFile(arg)
			default:
				return fmt.Sprintf("[Unknown: %s]", toolName)
			}
		})

		a.mu.Lock()
		a.messages = newMsgs
		a.mu.Unlock()

		a.appendChat("assistant", reply)

		a.busy = false
		a.sendBtn.SetEnabled(true)
		a.statusLabel.SetText("✅ 就绪")
	}()
}

func main() {
	initPaths()

	var apiKey string
	cfg := loadConfig()
	if cfg.APIKey != "" {
		apiKey = cfg.APIKey
	}
	if apiKey == "" {
		// Show API key dialog as a modal on top of the main window
		// First create the main window so dialog has a parent
		mw, err := walk.NewMainWindow()
		if err != nil {
			fmt.Println("Failed to create main window:", err)
			return
		}
		mw.SetTitle("Hermes Agent Lite - 设置 API Key")
		mw.SetBounds(walk.Rectangle{X: 0, Y: 0, Width: 450, Height: 180})
		mw.SetVisible(false)

		var lbl, _ = walk.NewLabel(mw)
		lbl.SetText("请输入你的 API Key:")
		lbl.SetBounds(walk.Rectangle{X: 15, Y: 15, Width: 400, Height: 20})

		var keyEntry *walk.LineEdit
		keyEntry, err = walk.NewLineEdit(mw)
		if err != nil {
			fmt.Println("Failed to create key entry:", err)
			return
		}
		keyEntry.SetBounds(walk.Rectangle{X: 15, Y: 40, Width: 400, Height: 25})

		var okBtn *walk.PushButton
		okBtn, err = walk.NewPushButton(mw)
		if err != nil {
			fmt.Println("Failed to create OK button:", err)
			return
		}
		okBtn.SetText("保存")
		okBtn.SetBounds(walk.Rectangle{X: 15, Y: 75, Width: 100, Height: 25})
		okBtn.Clicked().Attach(func() {
			k := keyEntry.Text()
			if k != "" {
				apiKey = k
				saveConfig(Config{APIKey: k})
			}
			mw.Close()
		})

		var cancelBtn *walk.PushButton
		cancelBtn, err = walk.NewPushButton(mw)
		if err != nil {
			fmt.Println("Failed to create cancel button:", err)
			return
		}
		cancelBtn.SetText("取消")
		cancelBtn.SetBounds(walk.Rectangle{X: 130, Y: 75, Width: 100, Height: 25})
		cancelBtn.Clicked().Attach(func() {
			mw.Close()
		})

		mw.SetVisible(true)
		mw.Run()

		if apiKey == "" {
			return
		}
	}

	a := &App{apiKey: apiKey}

	mw, err := walk.NewMainWindow()
	if err != nil {
		fmt.Println("Failed to create main window:", err)
		return
	}
	mw.SetTitle("Hermes Agent Lite v1.0")
	mw.SetBounds(walk.Rectangle{X: 100, Y: 100, Width: 900, Height: 650})

	// Status bar at bottom
	a.statusLabel, _ = walk.NewLabel(mw)
	a.statusLabel.SetText("✅ 就绪")
	a.statusLabel.SetBounds(walk.Rectangle{X: 10, Y: 595, Width: 200, Height: 25})

	// Send button top-left
	a.sendBtn, _ = walk.NewPushButton(mw)
	a.sendBtn.SetText("📨 发送")
	a.sendBtn.SetBounds(walk.Rectangle{X: 10, Y: 10, Width: 100, Height: 35})
	a.sendBtn.Clicked().Attach(a.send)

	// Settings button top-right
	var settingsBtn *walk.PushButton
	settingsBtn, _ = walk.NewPushButton(mw)
	settingsBtn.SetText("⚙️ 设置")
	settingsBtn.SetBounds(walk.Rectangle{X: 790, Y: 10, Width: 100, Height: 35})
	settingsBtn.Clicked().Attach(func() {
		setMW, err := walk.NewMainWindow()
		if err != nil {
			fmt.Println("Failed to create settings window:", err)
			return
		}
		setMW.SetTitle("设置 - API Key")
		setMW.SetBounds(walk.Rectangle{X: 200, Y: 200, Width: 500, Height: 150})

		var setLbl, _ = walk.NewLabel(setMW)
		setLbl.SetText("当前 API Key:")
		setLbl.SetBounds(walk.Rectangle{X: 10, Y: 10, Width: 200, Height: 20})

		var setEntry *walk.LineEdit
		setEntry, err = walk.NewLineEdit(setMW)
		if err != nil {
			fmt.Println("Failed:", err)
			return
		}
		setEntry.SetText(apiKey)
		setEntry.SetBounds(walk.Rectangle{X: 10, Y: 35, Width: 470, Height: 25})

		var setSaveBtn *walk.PushButton
		setSaveBtn, _ = walk.NewPushButton(setMW)
		setSaveBtn.SetText("保存")
		setSaveBtn.SetBounds(walk.Rectangle{X: 10, Y: 70, Width: 100, Height: 25})
		setSaveBtn.Clicked().Attach(func() {
			k := setEntry.Text()
			if k != "" {
				apiKey = k
				a.apiKey = k
				saveConfig(Config{APIKey: k})
				a.statusLabel.SetText("✅ 设置已更新")
			}
			setMW.Close()
		})

		var setCloseBtn *walk.PushButton
		setCloseBtn, _ = walk.NewPushButton(setMW)
		setCloseBtn.SetText("关闭")
		setCloseBtn.SetBounds(walk.Rectangle{X: 120, Y: 70, Width: 100, Height: 25})
		setCloseBtn.Clicked().Attach(func() {
			setMW.Close()
		})

		setMW.SetVisible(true)
	})

	// Chat text area (scrollable)
	a.chatEdit, _ = walk.NewTextEdit(mw)
	a.chatEdit.SetReadOnly(true)
	a.chatEdit.SetBounds(walk.Rectangle{X: 10, Y: 55, Width: 880, Height: 510})
	fnt, _ := walk.NewFont("Microsoft YaHei", 9, 0)
	a.chatEdit.SetFont(fnt)

	// Input entry at bottom-left (above status)
	a.inputEntry, _ = walk.NewLineEdit(mw)
	a.inputEntry.SetBounds(walk.Rectangle{X: 10, Y: 570, Width: 650, Height: 25})
	a.inputEntry.KeyDown().Attach(func(key walk.Key) {
		if key == walk.KeyReturn {
			a.send()
		}
	})

	a.appendChat("assistant", "欢迎使用 Hermes Agent Lite v1.0\n\n我是你的AI诊断助手，可以帮你：\n• 执行系统命令\n• 读写文件\n• 分析系统问题\n\n直接输入问题即可开始对话。")

	mw.SetVisible(true)
	a.mw = mw

	mw.Run()
}