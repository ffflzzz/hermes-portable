package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// chromeState holds the active debugging session info.
type chromeState struct {
	mu          sync.Mutex
	port        int
	wsURL       string
	conn        *websocket.Conn
	nextID      int
	debugDir    string
}

var chrome chromeState

// chromeFindExecutable returns the path to the system Chrome / Chromium.
func chromeFindExecutable() string {
	if runtime.GOOS == "windows" {
		locs := []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Users\` + whoami() + `\AppData\Local\Google\Chrome\Application\chrome.exe`,
		}
		for _, l := range locs {
			if fileExists(l) {
				return l
			}
		}
		return "chrome"
	}
	for _, c := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return "chrome"
}

func chromeUserDataDir() string {
	if runtime.GOOS == "windows" {
		return `\` + `Users\` + whoami() + `\AppData\Local\Google\Chrome\User Data`
	}
	return ""
}

// chromeLaunch starts the user's default Chrome with remote debugging enabled
// (non-headless, so a real visible window appears). Reuses the user's profile
// by launching a *temporary* copy of the profile to avoid locking the running
// Chrome. Returns the debugger URL.
func chromeLaunch(profile string, port int) string {
	if port == 0 {
		port = 9222
	}
	chrome.mu.Lock()
	if chrome.conn != nil && chrome.port == port {
		chrome.mu.Unlock()
		return fmt.Sprintf("already running on port %d (ws: %s)", port, chrome.wsURL)
	}
	chrome.mu.Unlock()

	exe := chromeFindExecutable()
	debugDir := profile
	if debugDir == "" {
		// Use an isolated debug profile so we don't clash with a running Chrome.
		tmp, _ := os.MkdirTemp("", "hermes-chrome-")
		debugDir = tmp
	}

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--no-first-run",
		"--no-default-browser-check",
		fmt.Sprintf("--user-data-dir=%s", debugDir),
		"about:blank",
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Launch detached so it keeps running after the tool returns.
		cmd = exec.Command("cmd", "/C", "start", "", exe)
		cmd.Args = append(cmd.Args, args...)
	} else {
		cmd = exec.Command(exe, args...)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Sprintf("[Error launching Chrome: %v]", err)
	}

	// Wait for the debugger endpoint to come up.
	versionURL := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	var wsURL string
	for i := 0; i < 40; i++ {
		if resp, err := http.Get(versionURL); err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var v struct {
				WebSSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			if json.Unmarshal(body, &v) == nil && v.WebSSocketDebuggerURL != "" {
				wsURL = v.WebSSocketDebuggerURL
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if wsURL == "" {
		return "[Error: Chrome started but debugger port did not respond]"
	}

	chrome.mu.Lock()
	chrome.port = port
	chrome.wsURL = wsURL
	chrome.debugDir = debugDir
	chrome.mu.Unlock()

	return fmt.Sprintf("Chrome launched (visible window). Debugger: %s", wsURL)
}

// chromeCDP sends a raw CDP command and returns the result.
func chromeCDP(method string, params map[string]interface{}) string {
	chrome.mu.Lock()
	wsURL := chrome.wsURL
	conn := chrome.conn
	if wsURL == "" {
		chrome.mu.Unlock()
		return "[Error: Chrome not launched. Call chrome_launch first.]"
	}
	if conn == nil {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			chrome.mu.Unlock()
			return fmt.Sprintf("[Error connecting to Chrome: %v]", err)
		}
		conn = c
		chrome.conn = c
	}
	id := chrome.nextID
	chrome.nextID++
	chrome.mu.Unlock()

	req := map[string]interface{}{
		"id":     id,
		"method": method,
	}
	if params != nil {
		req["params"] = params
	}
	data, _ := json.Marshal(req)
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Sprintf("[Error sending CDP command: %v]", err)
	}

	// Read until we get our response (skip events). Bound with a deadline so
	// a missing/stuck response cannot hang the whole chat forever.
	deadline := time.Now().Add(20 * time.Second)
	for {
		conn.SetReadDeadline(deadline)
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Sprintf("[Error reading CDP response (timed out?): %v]", err)
		}
		var resp struct {
			ID     int            `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(msg, &resp) != nil || resp.ID == 0 {
			continue // it's an event, ignore
		}
		if resp.Error != nil {
			return fmt.Sprintf("[CDP Error] %s", string(resp.Error))
		}
		out := string(resp.Result)
		if len(out) > 8000 {
			out = out[:8000] + "\n... (truncated)"
		}
		return out
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func whoami() string {
	if u, err := exec.Command("whoami").Output(); err == nil {
		s := string(u)
		if i := strings.LastIndex(s, "\\"); i >= 0 {
			return strings.TrimSpace(s[i+1:])
		}
		return strings.TrimSpace(s)
	}
	return ""
}
