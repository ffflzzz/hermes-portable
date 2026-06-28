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
)

// ─── Configuration ───────────────────────────────────────────────────────
const (
	APIBase   = "https://apihub.agnes-ai.com/v1"
	ModelName = "agnes-2.0-flash"
	AppDir    = ".hermes_lite"
)

var (
	configPath string
	sessionDir string
)

func initPaths() {
	exePath, _ := os.Executable()
	appDir := filepath.Join(filepath.Dir(exePath), AppDir)
	os.MkdirAll(appDir, 0755)
	configPath = filepath.Join(appDir, "config.json")
	sessionDir = filepath.Join(appDir, "sessions")
	os.MkdirAll(sessionDir, 0755)
}

// ─── Config ──────────────────────────────────────────────────────────────
type Config struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

func loadConfig() Config {
	var cfg Config
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &cfg)
	}
	if cfg.Model == "" {
		cfg.Model = ModelName
	}
	return cfg
}

func saveConfig(cfg Config) {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configPath, data, 0644)
}

// ─── Session ─────────────────────────────────────────────────────────────
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Session struct {
	Messages []Message `json:"messages"`
}

func loadSession() Session {
	p := filepath.Join(sessionDir, "default.json")
	var sess Session
	if data, err := os.ReadFile(p); err == nil {
		json.Unmarshal(data, &sess)
	}
	return sess
}

func saveSession(sess Session) {
	p := filepath.Join(sessionDir, "default.json")
	data, _ := json.MarshalIndent(sess, "", "  ")
	os.WriteFile(p, data, 0644)
}

// ─── System Info ─────────────────────────────────────────────────────────
func collectSysInfo() string {
	var info []string
	info = append(info, fmt.Sprintf("OS: %s %s", runtime.GOOS, runtime.GOARCH))
	info = append(info, "Hostname: "+mustExec("hostname"))
	info = append(info, "Working Dir: "+mustExec("pwd"))
	info = append(info, fmt.Sprintf("CPU Cores: %d", runtime.NumCPU()))

	if runtime.GOOS == "windows" {
		info = append(info, mustExec("systeminfo"))
	} else {
		info = append(info, mustExec("uname -a"))
		info = append(info, mustExec("df -h"))
		info = append(info, mustExec("free -h"))
	}
	return strings.Join(info, "\n")
}

func mustExec(cmd string) string {
	out, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("[%s failed: %v]", cmd, err)
	}
	s := string(out)
	if len(s) > 2000 {
		s = s[:2000]
	}
	return s
}

// ─── Tool Execution ──────────────────────────────────────────────────────
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

// ─── API Types ───────────────────────────────────────────────────────────
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

// ─── API Call ────────────────────────────────────────────────────────────
func callAPI(messages []Message, apiKey string, useTools bool) (string, []Message) {
	systemMsg := Message{
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

	allMessages := append([]Message{systemMsg}, messages...)

	for turn := 0; turn < 10; turn++ {
		payload := map[string]interface{}{
			"model":       ModelName,
			"messages":    allMessages,
			"temperature": 0.7,
			"max_tokens":  8192,
			"top_p":       0.9,
		}
		if useTools {
			payload["tools"] = []map[string]interface{}{
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "terminal",
						"description": "Execute a shell command. Returns stdout and stderr.",
						"parameters": map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{"command": map[string]interface{}{"type": "string", "description": "The shell command to execute"}},
							"required":   []string{"command"},
						},
					},
				},
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "read_file",
						"description": "Read the contents of a file.",
						"parameters": map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{"path": map[string]interface{}{"type": "string", "description": "File path to read"}},
							"required":   []string{"path"},
						},
					},
				},
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "write_file",
						"description": "Write content to a file.",
						"parameters": map[string]interface{}{
							"type":       "object",
							"properties": map[string]interface{}{"path": map[string]interface{}{"type": "string", "description": "File path to write"}, "content": map[string]interface{}{"type": "string", "description": "Content to write"}},
							"required":   []string{"path", "content"},
						},
					},
				},
			}
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
					allMessages = append(allMessages, Message{Role: "assistant", Content: msg.Content})

					for _, tc := range msg.ToolCalls {
						var args map[string]string
						json.Unmarshal([]byte(tc.Function.Arguments), &args)

						var result string
						switch tc.Function.Name {
						case "terminal":
							result = execTerminal(args["command"])
						case "read_file":
							result = execReadFile(args["path"])
						case "write_file":
							result = execWriteFile(args["path"], args["content"])
						default:
							result = fmt.Sprintf("[Unknown tool: %s]", tc.Function.Name)
						}

						allMessages = append(allMessages, Message{Role: "tool", Content: result})
					}
					continue
				}

				allMessages = append(allMessages, Message{Role: "assistant", Content: msg.Content})
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

// ─── CLI Mode ────────────────────────────────────────────────────────────
func banner() {
	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("  Hermes Agent Lite v1.0 - 精简版诊断助手")
	fmt.Println("============================================================")
	fmt.Println()
	fmt.Println("  功能: terminal | read_file | write_file")
	fmt.Println()
	fmt.Println("  命令:")
	fmt.Println("    /info     - 显示系统信息")
	fmt.Println("    /clear    - 清空对话")
	fmt.Println("    /reset    - 重置API Key")
	fmt.Println("    /quit     - 退出")
	fmt.Println()
	fmt.Println("  直接输入问题即可开始对话")
	fmt.Println()
}

func cliMode(apiKey string) {
	banner()

	session := loadSession()

	// Auto system info on first run
	if len(session.Messages) == 0 {
		fmt.Println("📋 正在收集系统信息...")
		sysInfo := collectSysInfo()
		fmt.Println(sysInfo)
		fmt.Println("\n正在发送给Hermes分析...\n")

		reply, msgs := callAPI(
			[]Message{{Role: "user", Content: "这是本机系统信息，请分析并给出初步诊断建议：\n" + sysInfo}},
			apiKey, true,
		)
		session.Messages = msgs
		saveSession(session)
		fmt.Printf("Hermes> %s\n\n", reply)
	}

	for {
		fmt.Print("你> ")
		var input string
		fmt.Scanln(&input)
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}

		switch input {
		case "/quit", "/exit":
			fmt.Println("再见！")
			return
		case "/clear":
			session.Messages = []Message{}
			saveSession(session)
			fmt.Println("对话已清空。\n")
		case "/reset":
			saveConfig(Config{})
			fmt.Println("API Key已清除。重启程序重新输入。")
			return
		case "/info":
			fmt.Println(collectSysInfo())
			fmt.Println()
		default:
			fmt.Print("思考中... ")
			reply, msgs := callAPI(session.Messages, apiKey, true)
			session.Messages = msgs
			saveSession(session)
			fmt.Printf("\nHermes> %s\n\n", reply)
		}
	}
}

// ─── Entry Point ────────────────────────────────────────────────────────
func main() {
	initPaths()

	cfg := loadConfig()

	if cfg.APIKey == "" {
		fmt.Print("首次运行，请输入你的 API Key:\n> ")
		var key string
		fmt.Scanln(&key)
		key = strings.TrimSpace(key)
		if key == "" {
			fmt.Println("未输入API Key，退出。")
			os.Exit(1)
		}
		cfg.APIKey = key
		saveConfig(cfg)
		fmt.Println("API Key已保存。\n")
	}

	cliMode(cfg.APIKey)
}
