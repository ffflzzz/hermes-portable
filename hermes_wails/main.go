package main

import (
	"bytes"
	"bufio"
	"context"
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

	wails "github.com/wailsapp/wails/v2"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"hermes_wails/frontend"
)

const (
	APIBase   = "https://apihub.agnes-ai.com/v1"
	ModelName = "agnes-2.0-flash"
	AppDir    = ".hermes_portable"
)

var (
	configPath string
	sessionDir string
)

type Config struct {
	APIKey string `json:"api_key"`
	Model  string `json:"model"`
}

func initPaths() {
	exePath, _ := os.Executable()
	d := filepath.Join(filepath.Dir(exePath), AppDir)
	os.MkdirAll(d, 0755)
	configPath = filepath.Join(d, "config.json")
	sessionDir = filepath.Join(d, "sessions")
	os.MkdirAll(sessionDir, 0755)
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

func sessionPath() string {
	return filepath.Join(sessionDir, "default.json")
}

func saveSession(msgs []ChatMsg) {
	data, _ := json.MarshalIndent(msgs, "", "  ")
	os.WriteFile(sessionPath(), data, 0644)
}

func loadSession() []ChatMsg {
	var msgs []ChatMsg
	if data, err := os.ReadFile(sessionPath()); err == nil {
		json.Unmarshal(data, &msgs)
	}
	return msgs
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

func execSearchFiles(pattern, searchPath, target string) string {
	if searchPath == "" {
		exePath, _ := os.Executable()
		searchPath = filepath.Dir(exePath)
	}
	if target == "files" {
		var matches []string
		_ = filepath.Walk(searchPath, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if matched, _ := filepath.Match(pattern, filepath.Base(p)); matched {
				matches = append(matches, p)
			}
			return nil
		})
		b, _ := json.Marshal(map[string]interface{}{"matches": matches, "count": len(matches)})
		return string(b)
	}
	cmd := exec.Command("grep", "-rli", pattern, searchPath)
	out, _ := cmd.CombinedOutput()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var matches []string
	for _, l := range lines {
		if l != "" {
			matches = append(matches, l)
		}
	}
	b, _ := json.Marshal(map[string]interface{}{"matches": matches, "count": len(matches)})
	return string(b)
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

// looksLikeImage detects base64 image payloads or data:image URIs so we can
// keep them out of the conversation history (the model can't read images).
func looksLikeImage(s string) bool {
	if strings.Contains(s, "data:image/") {
		return true
	}
	// CDP screenshots return large base64 blobs; flag very long alpha strings.
	if len(s) > 2000 {
		trimmed := strings.TrimSpace(s)
		if !strings.ContainsAny(trimmed, " \n\r\t{}[]()\"'") {
			// Heuristic: long whitespace-free blob is likely base64 image data.
			if len(trimmed) > 3000 && !strings.Contains(trimmed, "=") && !strings.Contains(trimmed, ":") {
				return true
			}
		}
	}
	return false
}

// isBlockedCDP rejects CDP methods that produce or read images/clipboard,
// which the model cannot consume and which cause retry-loops / hangs.
func isBlockedCDP(method string) bool {
	m := strings.ToLower(strings.TrimSpace(method))
	if m == "" {
		return false
	}
	// Substring match: any method touching screenshot/clipboard/snapshot/image.
	banned := []string{"screenshot", "snapshot", "clipboard", "cliphistory", "image", "headlessexperiment"}
	for _, b := range banned {
		if strings.Contains(m, b) {
			return true
		}
	}
	return false
}

func toolHandler(toolName, arg string) string {
	parts := strings.SplitN(arg, "\x00", 2)
	if len(parts) == 2 && toolName == "write_file" {
		return execWriteFile(parts[0], parts[1])
	}
	switch toolName {
	case "terminal":
		return execTerminal(arg)
	case "read_file":
		return execReadFile(arg)
	default:
		return fmt.Sprintf("[Unknown: %s]", toolName)
	}
}

const systemPrompt = "You are Hermes Agent, a diagnostic AI assistant with full system access.\n\n" +
	"Available tools:\n" +
	"- terminal: Execute shell commands. Parameter: \"command\" (string)\n" +
	"- read_file: Read file contents. Parameter: \"path\" (string)\n" +
	"- write_file: Write content to a file. Parameters: \"path\" (string), \"content\" (string)\n" +
	"- search_files: Search for files by name or content. Parameters: \"pattern\" (string), \"path\" (string, optional), \"target\" (\"files\" or \"content\")\n" +
	"- chrome_launch: Launch the user's real visible Chrome with debugging enabled. Call this first before controlling the browser.\n" +
	"- chrome_cdp: Send a Chrome DevTools Protocol command (method + params JSON) to drive the launched browser: navigate, click, type, read DOM, etc.\n\n" +
	"BROWSER RULES (critical):\n" +
	"- This model CANNOT read images. NEVER call Page.captureScreenshot, Page.captureSnapshot, or any clipboard/read-image CDP method. Such calls are BLOCKED and will be rejected. If you need page content, use chrome_cdp with Runtime.evaluate returning document.body.innerText or DOM text as a string.\n" +
	"- To read a page: chrome_cdp method=Runtime.evaluate params={\"expression\":\"document.body.innerText\"}\n" +
	"- To open a page: chrome_cdp method=Page.navigate params={\"url\":\"https://...\"}\n" +
	"- After navigation, wait briefly then read text. Do not assume screenshots.\n" +
	"- If a tool call fails, do NOT retry the same failing call more than once. Adapt or tell the user.\n\n" +
	"SPEED RULES:\n" +
	"- Be decisive. Plan one concrete step at a time. Avoid long chains of redundant tool calls.\n" +
	"- Prefer a single Runtime.evaluate that returns the needed text over many small calls.\n\n" +
	"When the user asks a question:\n" +
	"1. Think step by step about what tools you need\n" +
	"2. Call tools as needed to gather information and solve the problem\n" +
	"3. Return clear, actionable results\n" +
	"4. For non-technical users, explain in simple terms\n\n" +
	"Always use the local system's shell (bash on Linux, PowerShell/cmd on Windows).\n" +
	"Be thorough but concise in your responses.\n\n" +
	"IMPORTANT - reply style for a non-technical chat user:\n" +
	"- Write like a friendly human support agent chatting in a messaging app, not a document.\n" +
	"- Do NOT use Markdown formatting: no # headings, no bold, no italics, no bullet lists, no quote marks, no code fences.\n" +
	"- Use plain line breaks to separate thoughts. For short step lists, write them as 第一步：... 第二步：... in normal text.\n" +
	"- Only wrap an exact command the user must copy/paste in single backticks, and keep it on its own line.\n" +
	"- Keep it warm, concise and easy to read on a phone-like window."

func callAPI(ctx context.Context, messages []ChatMsg, apiKey string, onEvent func(string, string)) (string, []ChatMsg) {
	systemMsg := ChatMsg{Role: "system", Content: systemPrompt}
	allMessages := append([]ChatMsg{systemMsg}, messages...)

	for turn := 0; turn < 10; turn++ {
		select {
		case <-ctx.Done():
			return "[cancelled]", allMessages
		default:
		}

		payload := map[string]interface{}{
			"model":       ModelName,
			"messages":    allMessages,
			"temperature": 0.7,
			"max_tokens":  2048,
			"top_p":       0.9,
			"stream":      true,
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
				{"type": "function", "function": map[string]interface{}{
					"name":        "search_files",
					"description": "Search for files by name pattern or content.",
					"parameters": map[string]interface{}{
						"type": "object", "properties": map[string]interface{}{
							"pattern": map[string]interface{}{"type": "string", "description": "Glob pattern for files or regex for content"},
							"path":    map[string]interface{}{"type": "string", "description": "Directory to search (optional)"},
							"target":  map[string]interface{}{"type": "string", "enum": []string{"files", "content"}, "description": "Search in filenames or content"},
						}, 						"required": []string{"pattern"},
					},
				}},
				{"type": "function", "function": map[string]interface{}{
					"name":        "chrome_launch",
					"description": "Launch the user's real (visible, non-headless) Chrome with remote debugging enabled so it can be controlled via CDP. Returns the debugger WebSocket URL. Call this before any chrome_cdp call.",
					"parameters": map[string]interface{}{
						"type": "object", "properties": map[string]interface{}{
							"profile": map[string]interface{}{"type": "string", "description": "Optional user-data-dir path. Leave empty to use a temporary isolated profile."},
						}, "required": []string{},
					},
				}},
				{"type": "function", "function": map[string]interface{}{
					"name":        "chrome_cdp",
					"description": "Send a raw Chrome DevTools Protocol command to the launched Chrome. method is like 'Page.navigate', 'Runtime.evaluate', 'DOM.querySelector', 'Input.insertText'. params is a JSON object. Returns the CDP result.",
					"parameters": map[string]interface{}{
						"type": "object", "properties": map[string]interface{}{
							"method": map[string]interface{}{"type": "string", "description": "CDP method, e.g. Page.navigate"},
							"params": map[string]interface{}{"type": "string", "description": "JSON string of CDP params, e.g. {\"url\":\"https://example.com\"}"},
						}, "required": []string{"method"},
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
			return fmt.Sprintf("network error: %v", err), allMessages
		}

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 401 {
				return "API Key invalid", allMessages
			} else if resp.StatusCode == 429 {
				return "too many requests, slow down", allMessages
			}
			return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)[:200]), allMessages
		}

		finalContent, toolCalls, _, streamErr := parseStream(resp, onEvent)
		resp.Body.Close()
		if streamErr != "" {
			return streamErr, allMessages
		}

		if len(toolCalls) > 0 {
			allMessages = append(allMessages, ChatMsg{Role: "assistant", Content: finalContent})
			if onEvent != nil {
				onEvent("tool_start", finalContent)
			}
			for _, tc := range toolCalls {
				var args map[string]string
				json.Unmarshal([]byte(tc.Arguments), &args)
				if onEvent != nil {
					onEvent("tool_call", fmt.Sprintf("%s: %s", tc.Name, tc.Arguments))
				}
				var result string
				switch tc.Name {
				case "terminal":
					result = toolHandler("terminal", args["command"])
				case "read_file":
					result = toolHandler("read_file", args["path"])
				case "write_file":
					result = toolHandler("write_file", args["path"]+"\x00"+args["content"])
				case "search_files":
					result = execSearchFiles(args["pattern"], args["path"], args["target"])
				case "chrome_launch":
					result = chromeLaunch(args["profile"], 0)
				case "chrome_cdp":
					var cdpParams map[string]interface{}
					if args["params"] != "" {
						json.Unmarshal([]byte(args["params"]), &cdpParams)
					}
					// Hard-block image/screenshot/clipboard methods: the model
					// cannot read images, and retrying these just hangs the chat.
					if isBlockedCDP(args["method"]) {
						result = "[blocked] This model cannot read images or the clipboard. Use Runtime.evaluate to read page text instead (e.g. document.body.innerText)."
					} else {
						result = chromeCDP(args["method"], cdpParams)
					}
				default:
					result = fmt.Sprintf("[Unknown tool: %s]", tc.Name)
				}
				if onEvent != nil {
					onEvent("tool_result", result)
				}
				if looksLikeImage(result) {
					result = "[image data ignored: this model does not support image input]"
				}
				allMessages = append(allMessages, ChatMsg{Role: "tool", Content: result})
			}
			continue
		}

		allMessages = append(allMessages, ChatMsg{Role: "assistant", Content: finalContent})
		return finalContent, allMessages
	}

	return "reached max turns", allMessages
}

// streamToolCall accumulates a streamed tool call (which arrives in pieces).
type streamToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// parseStream reads an SSE response, emits token events for live display, and
// returns the final text plus any accumulated tool calls.
func parseStream(resp *http.Response, onEvent func(string, string)) (string, []streamToolCall, bool, string) {
	reader := bufio.NewReader(resp.Body)
	var finalContent strings.Builder
	var toolCalls []streamToolCall
	toolIndex := map[int]*streamToolCall{}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			break
		}
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			if err != nil {
				break
			}
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
					Role string `json:"role"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			if err != nil {
				break
			}
			continue
		}
		if len(chunk.Choices) == 0 {
			if err != nil {
				break
			}
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			finalContent.WriteString(delta.Content)
			if onEvent != nil {
				onEvent("token", delta.Content)
			}
		}
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			st, ok := toolIndex[idx]
			if !ok {
				st = &streamToolCall{}
				toolIndex[idx] = st
				toolCalls = append(toolCalls, *st)
				st = &toolCalls[len(toolCalls)-1]
			}
			if tc.ID != "" {
				st.ID = tc.ID
			}
			if tc.Function.Name != "" {
				st.Name = tc.Function.Name
			}
			st.Arguments += tc.Function.Arguments
		}
		if err != nil {
			break
		}
	}
	return finalContent.String(), toolCalls, true, ""
}

// ─── App ─────────────────────────────────────────────────────────────────
type ChatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type App struct {
	ctx      context.Context
	mu       sync.Mutex
	apiKey   string
	messages []ChatMsg
	busy     bool
	cancel   context.CancelFunc
}

func NewApp() *App {
	return &App{}
}

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:  "Hermes Portable",
		Width:  900,
		Height: 680,
		AssetServer: &assetserver.Options{
			Assets: frontend.Assets(),
		},
		BackgroundColour: &options.RGBA{R: 15, G: 20, B: 25, A: 255},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	initPaths()
	cfg := loadConfig()
	a.apiKey = cfg.APIKey
	a.messages = loadSession()
	if a.apiKey == "" {
		wailsruntime.EventsEmit(ctx, "need_apikey", true)
	}
}

func (a *App) domReady(ctx context.Context) {
	a.mu.Lock()
	hist := make([]ChatMsg, len(a.messages))
	copy(hist, a.messages)
	a.mu.Unlock()
	wailsruntime.EventsEmit(ctx, "history", hist)
}
func (a *App) beforeClose(ctx context.Context) bool { return false }
func (a *App) shutdown(ctx context.Context)   {}

func (a *App) GetAPIKeyStatus() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.apiKey != ""
}

func (a *App) SetAPIKey(key string) {
	a.mu.Lock()
	a.apiKey = key
	saveConfig(Config{APIKey: key, Model: ModelName})
	a.mu.Unlock()
	wailsruntime.EventsEmit(a.ctx, "need_apikey", false)
}

func (a *App) LoadHistory() []ChatMsg {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.messages
}

func (a *App) SendMessage(userInput string) string {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return ""
	}
	if a.apiKey == "" {
		a.mu.Unlock()
		wailsruntime.EventsEmit(a.ctx, "need_apikey", true)
		return ""
	}
	a.busy = true
	key := a.apiKey
	a.mu.Unlock()

	wailsruntime.EventsEmit(a.ctx, "status", "thinking")
	wailsruntime.EventsEmit(a.ctx, "user_msg", userInput)

	a.mu.Lock()
	a.messages = append(a.messages, ChatMsg{Role: "user", Content: userInput})
	msgsCopy := make([]ChatMsg, len(a.messages))
	copy(msgsCopy, a.messages)
	a.mu.Unlock()

	ctx, cancel := context.WithCancel(a.ctx)
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()

	reply, newMsgs := callAPI(ctx, msgsCopy, key, func(kind, payload string) {
		wailsruntime.EventsEmit(a.ctx, "tool_event", map[string]string{"kind": kind, "payload": payload})
	})

	a.mu.Lock()
	a.messages = newMsgs
	a.busy = false
	saveSession(newMsgs)
	a.mu.Unlock()

	wailsruntime.EventsEmit(a.ctx, "assistant_msg", reply)
	wailsruntime.EventsEmit(a.ctx, "status", "ready")
	return reply
}

func (a *App) ClearHistory() {
	a.mu.Lock()
	a.messages = []ChatMsg{}
	saveSession(a.messages)
	a.mu.Unlock()
}

func (a *App) Stop() {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.busy = false
	a.mu.Unlock()
	wailsruntime.EventsEmit(a.ctx, "status", "ready")
}

func (a *App) CollectSystemInfo() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("OS: %s %s\n", runtime.GOOS, runtime.GOARCH))
	if host, err := os.Hostname(); err == nil {
		b.WriteString(fmt.Sprintf("Hostname: %s\n", host))
	}
	b.WriteString(fmt.Sprintf("CPU Cores: %d\n", runtime.NumCPU()))
	b.WriteString(fmt.Sprintf("Time: %s\n", time.Now().Format(time.RFC1123)))
	return b.String()
}
