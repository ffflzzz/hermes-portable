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

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	APIBase   = "https://apihub.agnes-ai.com/v1"
	ModelName = "agnes-2.0-flash"
	AppDir    = ".hermes_lite"
)

var configPath, sessionDir string

func initPaths() {
	exePath, _ := os.Executable()
	d := filepath.Join(filepath.Dir(exePath), AppDir)
	os.MkdirAll(d, 0755)
	configPath = filepath.Join(d, "config.json")
	sessionDir = filepath.Join(d, "sessions")
	os.MkdirAll(sessionDir, 0755)
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
}

// ─── Tools ───────────────────────────────────────────────────────────────
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
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("[Error: %v]", err)
	}
	if len(content) > 50000 {
		content = content[:50000]
	}
	return string(content)
}

func execWriteFile(path, content string) string {
	if !filepath.IsAbs(path) {
		exePath, _ := os.Executable()
		path = filepath.Join(filepath.Dir(exePath), path)
	}
	os.MkdirAll(filepath.Dir(path), 0755)
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		return fmt.Sprintf("[Error: %v]", err)
	}
	return fmt.Sprintf("OK: wrote %d bytes", len(content))
}

// ─── API ─────────────────────────────────────────────────────────────────
type ToolCall struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type APIResponse struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

func callAPI(messages []ChatMsg, apiKey string, toolFn func(string, string) string) (string, []ChatMsg) {
	systemMsg := ChatMsg{
		Role: "system",
		Content: `You are Hermes Agent Lite, a diagnostic AI assistant with full system access.

Available tools:
- terminal: Execute shell commands. Parameter: "command" (string)
- read_file: Read file contents. Parameter: "path" (string)
- write_file: Write content to file. Parameters: "path" (string), "content" (string)

When the user asks a question:
1. Think about what tools you need
2. Call tools as needed
3. Return clear, actionable results
4. For non-technical users, explain in simple terms

Always use the local system's shell. Be thorough but concise.`,
	}

	allMessages := append([]ChatMsg{systemMsg}, messages...)

	for turn := 0; turn < 10; turn++ {
		payload := map[string]interface{}{
			"model":       ModelName,
			"messages":    allMessages,
			"temperature": 0.7,
			"max_tokens":  8192,
			"top_p":       0.9,
			"tools": []map[string]interface{}{
				{"type": "function", "function": map[string]interface{}{
					"name":        "terminal",
					"description": "Execute a shell command. Returns stdout and stderr.",
					"parameters": map[string]interface{}{
						"type": "object", "properties": map[string]interface{}{
							"command": map[string]interface{}{"type": "string", "description": "The shell command to execute"},
						}, "required": []string{"command"},
					},
				}},
				{"type": "function", "function": map[string]interface{}{
					"name":        "read_file",
					"description": "Read the contents of a file.",
					"parameters": map[string]interface{}{
						"type": "object", "properties": map[string]interface{}{
							"path": map[string]interface{}{"type": "string", "description": "File path to read"},
						}, "required": []string{"path"},
					},
				}},
				{"type": "function", "function": map[string]interface{}{
					"name":        "write_file",
					"description": "Write content to a file.",
					"parameters": map[string]interface{}{
						"type": "object", "properties": map[string]interface{}{
							"path":    map[string]interface{}{"type": "string", "description": "File path to write"},
							"content": map[string]interface{}{"type": "string", "description": "Content to write"},
						}, "required": []string{"path", "content"},
					},
				}},
			},
		}

		jsonData, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", APIBase+"/chat/completions", bytes.NewBuffer(jsonData))
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Sprintf("❌ 网络错误: %v", err), allMessages
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 200 {
			var apiResp APIResponse
			json.Unmarshal(body, &apiResp)

			if len(apiResp.Choices) > 0 {
				msg := apiResp.Choices[0].Message

				if len(msg.ToolCalls) > 0 {
					allMessages = append(allMessages, ChatMsg{Role: "assistant", Content: msg.Content})

					for _, tc := range msg.ToolCalls {
						var args map[string]string
						json.Unmarshal([]byte(tc.Function.Arguments), &args)

						var result string
						switch tc.Function.Name {
						case "terminal":
							result = toolFn("terminal", args["command"])
						case "read_file":
							result = toolFn("read_file", args["path"])
						case "write_file":
							result = toolFn("write_file", args["path"]+"\x00"+args["content"])
						default:
							result = fmt.Sprintf("[Unknown tool: %s]", tc.Function.Name)
						}

						allMessages = append(allMessages, ChatMsg{Role: "tool", Content: result})
					}
					continue
				}

				allMessages = append(allMessages, ChatMsg{Role: "assistant", Content: msg.Content})
				return msg.Content, allMessages
			}
		} else if resp.StatusCode == 401 {
			return "❌ API Key无效", allMessages
		} else if resp.StatusCode == 429 {
			return "⏳ 请求太频繁，请稍后再试", allMessages
		} else {
			return fmt.Sprintf("❌ HTTP %d", resp.StatusCode), allMessages
		}
	}

	return "⚠️ 达到最大轮次限制", allMessages
}

// ─── GUI ─────────────────────────────────────────────────────────────────
type chatWindow struct {
	apiKey      string
	messages    []ChatMsg
	log         []string
	mu          sync.Mutex
	textArea    *widget.Entry
	statusLabel *widget.Label
	sendBtn     *widget.Button
	busy        bool
}

func (cw *chatWindow) appendLog(role, content string) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	now := cw.now()
	var prefix string
	switch role {
	case "user":
		prefix = fmt.Sprintf("[%s] 👤 你: ", now)
	case "assistant":
		prefix = fmt.Sprintf("[%s] 🤖 Hermes: ", now)
	case "tool":
		prefix = fmt.Sprintf("[%s] 🔧 工具: ", now)
	default:
		prefix = fmt.Sprintf("[%s] ", now)
	}

	line := prefix + strings.ReplaceAll(content, "\n", "\n  ")
	cw.log = append(cw.log, line)

	// Keep last 500 lines
	if len(cw.log) > 500 {
		cw.log = cw.log[len(cw.log)-500:]
	}

	cw.textArea.SetText(strings.Join(cw.log, "\n"))
	cw.textArea.ScrollToBottom()
}

func (cw *chatWindow) now() string {
	return time.Now().Format("15:04:05")
}

func (cw *chatWindow) send() {
	if cw.busy {
		return
	}
	input := strings.TrimSpace(cw.textArea.Text)
	if input == "" {
		return
	}

	cw.busy = true
	cw.sendBtn.Disable()
	cw.statusLabel.SetText("⏳ 思考中...")

	// Extract user message (last line that starts with 👤)
	lines := strings.Split(input, "\n")
	userMsg := lines[len(lines)-1]

	cw.appendLog("user", userMsg)

	go func() {
		// Convert log to messages for API
		var apiMessages []ChatMsg
		for _, line := range cw.messages {
			apiMessages = append(apiMessages, line)
		}
		apiMessages = append(apiMessages, ChatMsg{Role: "user", Content: userMsg})

		reply, msgs := callAPI(apiMessages, cw.apiKey, func(toolName, arg string) string {
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
			case "write_file":
				return "[Invalid args]"
			default:
				return fmt.Sprintf("[Unknown: %s]", toolName)
			}
		})

		cw.mu.Lock()
		cw.messages = msgs
		cw.mu.Unlock()

		cw.appendLog("assistant", reply)

		cw.mu.Lock()
		cw.busy = false
		cw.mu.Unlock()
		cw.sendBtn.Enable()
		cw.statusLabel.SetText("✅ 就绪")
	}()
}

func runGUI(apiKey string) {
	a := app.New()
	w := a.NewWindow("Hermes Agent Lite")
	w.SetMaster()
	w.Resize(fyne.NewSize(800, 600))

	cw := &chatWindow{apiKey: apiKey}

	// Welcome message
	cw.appendLog("assistant", "欢迎使用 Hermes Agent Lite v1.0\n\n我是你的AI诊断助手，可以帮你：\n• 执行系统命令\n• 读写文件\n• 分析系统问题\n\n直接输入问题即可开始对话。")

	// Chat display (read-only, auto-scrolling)
	display := widget.NewMultiLineEntry()
	display.Disable()
	display.Wrapping = fyne.TextWrapWord
	display.SetText("") // Start fresh, we'll append via appendLog

	// Override appendLog to use display instead
	cw.textArea = display

	// Input area
	input := widget.NewEntry()
	input.SetPlaceHolder("输入问题... (例如: 查看C盘剩余空间)")
	input.OnSubmitted = func(text string) {
		cw.appendLog("user", text)

		go func() {
			cw.busy = true
			cw.sendBtn.Disable()
			cw.statusLabel.SetText("⏳ 思考中...")

			var apiMessages []ChatMsg
			apiMessages = append(apiMessages, ChatMsg{Role: "user", Content: text})

			reply, msgs := callAPI(apiMessages, cw.apiKey, func(toolName, arg string) string {
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

			cw.mu.Lock()
			cw.messages = msgs
			cw.busy = false
			cw.mu.Unlock()

			cw.appendLog("assistant", reply)
			cw.sendBtn.Enable()
			cw.statusLabel.SetText("✅ 就绪")
		}()
	}

	// Buttons
	cw.sendBtn = widget.NewButton("发送", func() {
		input.OnSubmitted(input.Text)
	})

	toolbar := container.NewHBox(
		widget.NewButtonWithIcon("清空", theme.DeleteIcon(), func() {
			cw.messages = []ChatMsg{}
			cw.log = []string{}
			display.SetText("")
			cw.appendLog("assistant", "对话已清空。")
		}),
		widget.NewButtonWithIcon("API", theme.AccountIcon(), func() {
			keyEntry := widget.NewEntry()
			keyEntry.SetPlaceHolder("输入新的API Key...")
			form := container.NewVBox(
				widget.NewLabel("设置 API Key:"),
				keyEntry,
				container.NewHBox(
					widget.NewButton("保存", func() {
						cw.apiKey = keyEntry.Text
						saveConfig(Config{APIKey: keyEntry.Text})
						cw.statusLabel.SetText("✅ API Key已保存")
						modalDlg.Close()
					}),
					widget.NewButton("取消", func() { modalDlg.Close() }),
				),
			)
			modalDlg := widget.NewModalForm("设置 API Key", form)
			modalDlg.Show()
		}),
		container.NewSpacer(),
		cw.statusLabel,
	)

	content := container.NewBorder(
		toolbar,
		container.NewHBox(input, cw.sendBtn),
		nil, nil,
		display,
	)

	w.SetContent(content)
	w.ShowAndRun()
}
