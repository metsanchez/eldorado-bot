package eldorado

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"eldorado-bot/internal/logger"
)

type BrowserLoginResult struct {
	CookieString string
	XSRFToken    string
}

type patchrightResult struct {
	Cookies   string `json:"cookies"`
	XSRFToken string `json:"xsrf_token"`
	Error     string `json:"error"`
}

// BrowserLogin calls the patchright-based Python helper script to perform
// login. Patchright patches the browser to bypass Cloudflare Turnstile detection
// that blocks CDP-based tools like chromedp.
func BrowserLogin(ctx context.Context, baseURL, email, password string, log *logger.Logger) (*BrowserLoginResult, error) {
	log.Infof("browser-login: starting patchright login for %s", email)

	scriptPath := findLoginScript()
	if scriptPath == "" {
		return nil, fmt.Errorf("browser_login.py script not found")
	}
	log.Infof("browser-login: using script: %s", scriptPath)

	pythonBin := findPython()
	log.Infof("browser-login: using python: %s", pythonBin)

	args := []string{scriptPath, baseURL, email, password}

	cmd := exec.CommandContext(ctx, pythonBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")

	log.Infof("browser-login: launching patchright subprocess...")

	if err := cmd.Run(); err != nil {
		stderrStr := stderr.String()
		// Log stderr lines (patchright progress)
		for _, line := range strings.Split(stderrStr, "\n") {
			if strings.TrimSpace(line) != "" {
				log.Infof("  %s", line)
			}
		}
		return nil, fmt.Errorf("patchright login failed: %w\nstderr: %s", err, lastLines(stderrStr, 10))
	}

	// Log stderr (progress messages)
	for _, line := range strings.Split(stderr.String(), "\n") {
		if strings.TrimSpace(line) != "" {
			log.Infof("  %s", line)
		}
	}

	// Parse JSON result from stdout
	var result patchrightResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse patchright output: %w (raw: %s)", err, truncateStr(stdout.String(), 200))
	}

	if result.Error != "" {
		return nil, fmt.Errorf("patchright login error: %s", result.Error)
	}

	if result.Cookies == "" {
		return nil, fmt.Errorf("patchright returned empty cookies")
	}

	log.Infof("browser-login: login successful! cookie length=%d", len(result.Cookies))

	return &BrowserLoginResult{
		CookieString: result.Cookies,
		XSRFToken:    result.XSRFToken,
	}, nil
}

// SendChatMessage launches a browser to send a message to a buyer on a boosting request.
func SendChatMessage(ctx context.Context, requestID, messageText, imagePath string, log *logger.Logger) error {
	scriptPath := findScript("send_chat_message.py")
	if scriptPath == "" {
		return fmt.Errorf("send_chat_message.py script not found")
	}

	pythonBin := findPython()
	args := []string{scriptPath, requestID, messageText}
	if imagePath != "" {
		absImage := imagePath
		if !filepath.IsAbs(imagePath) {
			if wd, err := os.Getwd(); err == nil {
				absImage = filepath.Join(wd, imagePath)
			}
		}
		args = append(args, absImage)
	}

	cmd := exec.CommandContext(ctx, pythonBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")

	log.Infof("chat-message: sending to request %s...", requestID[:min(len(requestID), 12)])

	if err := cmd.Run(); err != nil {
		for _, line := range strings.Split(stderr.String(), "\n") {
			if strings.TrimSpace(line) != "" {
				log.Infof("  %s", line)
			}
		}
		return fmt.Errorf("send chat message failed: %w", err)
	}

	for _, line := range strings.Split(stderr.String(), "\n") {
		if strings.TrimSpace(line) != "" {
			log.Infof("  %s", line)
		}
	}

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return fmt.Errorf("parse chat result: %w (raw: %s)", err, truncateStr(stdout.String(), 200))
	}
	if result.Error != "" {
		return fmt.Errorf("chat message error: %s", result.Error)
	}

	log.Infof("chat-message: sent successfully")
	return nil
}

func findScript(name string) string {
	candidates := []string{
		"scripts/" + name,
		name,
	}
	if execPath, err := os.Executable(); err == nil {
		dir := filepath.Dir(execPath)
		candidates = append(candidates,
			filepath.Join(dir, "scripts", name),
			filepath.Join(dir, "..", "scripts", name),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "scripts", name),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func findLoginScript() string {
	candidates := []string{
		"scripts/browser_login.py",
		"browser_login.py",
	}

	// Try relative to executable
	if execPath, err := os.Executable(); err == nil {
		dir := filepath.Dir(execPath)
		candidates = append(candidates,
			filepath.Join(dir, "scripts", "browser_login.py"),
			filepath.Join(dir, "..", "scripts", "browser_login.py"),
		)
	}

	// Try relative to working directory
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "scripts", "browser_login.py"),
		)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func findPython() string {
	// macOS system python3
	if runtime.GOOS == "darwin" {
		paths := []string{
			"/Library/Developer/CommandLineTools/usr/bin/python3",
			"/usr/local/bin/python3",
			"/usr/bin/python3",
			"/opt/homebrew/bin/python3",
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	if p, err := exec.LookPath("python3"); err == nil {
		return p
	}
	return "python3"
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
