package eldorado

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"eldorado-bot/internal/logger"
)

const talkJsAppID = "49mLECOW"
const talkJsBaseURL = "https://app.talkjs.com"
const talkJsUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0"

func isHex20(s string) bool {
	if len(s) != 20 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func sliceContains(sl []string, v string) bool {
	for _, x := range sl {
		if x == v {
			return true
		}
	}
	return false
}

// OneOnOneID computes TalkJS conversation ID from two user IDs (SHA1 of sorted JSON, first 20 hex chars).
func OneOnOneID(user1, user2 string) string {
	if user1 == "" || user2 == "" {
		return ""
	}
	ids := []string{strings.TrimSpace(user1), strings.TrimSpace(user2)}
	sort.Strings(ids)
	payload, _ := json.Marshal(ids)
	h := sha1.Sum(payload)
	return strings.ToLower(hex.EncodeToString(h[:])[:20])
}

// TalkJsSay sends a text message via TalkJS say API (no browser).
// Tries: 1) app.talkjs.com say, 2) api.talkjs.com REST v1 messages.
// altConvID: optional oneOnOneId(buyer, seller) - try this first if provided.
func (c *Client) TalkJsSay(ctx context.Context, conversationID, messageText, token, nymId string, log *logger.Logger) error {
	return c.TalkJsSayWithAlt(ctx, conversationID, "", messageText, token, nymId, log)
}

// TalkJsSayWithAlt tries altConvID (oneOnOneId) first, then conversationID.
func (c *Client) TalkJsSayWithAlt(ctx context.Context, conversationID, altConvID, messageText, token, nymId string, log *logger.Logger) error {
	if token == "" || nymId == "" {
		return fmt.Errorf("TalkJS token and nymId required for API send")
	}

	convID := strings.TrimSpace(conversationID)
	altID := strings.TrimSpace(altConvID)
	if convID == "" && altID == "" {
		return fmt.Errorf("conversation ID required")
	}

	idempotencyKey, _ := randomHex(12)
	sessionID, _ := randomHex(32)
	sessionID = formatUUID(sessionID)

	// entityTree: TalkJS bazen çok satırlı/emoji mesajlarda 500 veriyor - önce plain dene
	entityText := messageText
	if len(entityText) > 500 {
		entityText = entityText[:500]
	}

	body := map[string]any{
		"idempotencyKey": idempotencyKey,
		"entityTree":     []string{entityText},
		"received":       false,
		"custom":         map[string]string{},
		"nymId":          nymId,
		"attachment":     nil,
		"location":       nil,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal say body: %w", err)
	}

	clientDate := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	headers := map[string]string{
		"Accept":                "application/json",
		"Content-Type":          "application/json",
		"Authorization":         "bearer " + strings.TrimSpace(token),
		"Origin":                talkJsBaseURL,
		"Referer":               talkJsBaseURL + "/",
		"User-Agent":            talkJsUserAgent,
		"x-talkjs-client-build": "frontend-release-855acf7",
		"x-talkjs-client-date":  clientDate,
	}

	// Build IDs to try. TalkJS one-on-one uses 20 hex chars (SHA1).
	// 1) Eldorado talkJsConversationID when already 20 hex (they may return real ID)
	// 2) oneOnOneId(buyer,seller) - TalkJS canonical format for 1:1
	// 3) UUID variants (Eldorado might use custom per-order IDs)
	idsToTry := []string{}
	convTrim := strings.TrimSpace(convID)
	if altID != "" {
		idsToTry = append(idsToTry, altID)
	}
	if convTrim != "" {
		if isHex20(convTrim) && !sliceContains(idsToTry, convTrim) {
			idsToTry = append(idsToTry, convTrim)
		}
		noDash := strings.ToLower(strings.ReplaceAll(convTrim, "-", ""))
		if len(noDash) >= 20 {
			twenty := noDash[:20]
			if isHex20(twenty) && !sliceContains(idsToTry, twenty) {
				idsToTry = append(idsToTry, twenty)
			}
		}
		if len(noDash) > 20 && !sliceContains(idsToTry, noDash) {
			idsToTry = append(idsToTry, noDash)
		}
		if !sliceContains(idsToTry, convTrim) {
			idsToTry = append(idsToTry, convTrim)
		}
	}

	var lastErr error
	for i, id := range idsToTry {
		path := fmt.Sprintf("/api/v0/%s/say/%s/?sessionId=%s", talkJsAppID, id, sessionID)
		fullURL := talkJsBaseURL + path

		bodyOut, err := c.curlRequestTalkJs(ctx, "POST", fullURL, jsonBody, headers)
		if err != nil {
			lastErr = err
			log.Infof("TalkJS say attempt (convId=%s): %v", id, err)
			if strings.Contains(err.Error(), "500") && i == 0 {
				time.Sleep(3 * time.Second)
				bodyOut, err = c.curlRequestTalkJs(ctx, "POST", fullURL, jsonBody, headers)
				if err == nil {
					var retryResp struct {
						Ok    string `json:"ok"`
						Error string `json:"error"`
					}
					_ = json.Unmarshal(bodyOut, &retryResp)
					if retryResp.Ok != "" {
						log.Infof("chat-message: sent via TalkJS API (retry, msgId=%s)", retryResp.Ok)
						return nil
					}
				}
			}
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "403") ||
				strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "500") {
				continue
			}
			return fmt.Errorf("TalkJS say: %w", err)
		}

		var resp struct {
			Ok      string `json:"ok"`
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(bodyOut, &resp)
		if resp.Ok != "" {
			log.Infof("chat-message: sent via TalkJS API (msgId=%s)", resp.Ok)
			return nil
		}
		if resp.Error != "" || resp.Message != "" {
			lastErr = fmt.Errorf("%s %s", resp.Error, resp.Message)
			continue
		}

		log.Infof("chat-message: sent via TalkJS API (convId=%s)", id)
		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("TalkJS say failed for all ID formats: %w", lastErr)
	}
	return fmt.Errorf("TalkJS say failed")
}

func (c *Client) curlRequestTalkJs(ctx context.Context, method, fullURL string, jsonBody []byte, extraHeaders map[string]string) ([]byte, error) {
	args := []string{
		"-sL", "--compressed",
		"--max-time", "15",
		"-X", method,
		"-H", "Accept: application/json",
		"-H", "Content-Type: application/json",
		"-H", "User-Agent: " + talkJsUserAgent,
		"-H", "Origin: " + talkJsBaseURL,
		"-H", "Referer: " + talkJsBaseURL + "/",
		"-w", "\n__HTTP_CODE__:%{http_code}",
	}

	for k, v := range extraHeaders {
		if v != "" {
			args = append(args, "-H", k+": "+v)
		}
	}
	if jsonBody != nil {
		args = append(args, "-d", string(jsonBody))
	}
	args = append(args, fullURL)

	return c.execCurl(ctx, args)
}

// talkJsTokenStorage path (storage/talkjs_token.json)
func talkJsTokenPath() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "storage", "talkjs_token.json")
}

// LoadTalkJsTokenFromStorage loads token from storage/talkjs_token.json if valid (5min buffer before expiry).
func LoadTalkJsTokenFromStorage(log *logger.Logger) string {
	data, err := os.ReadFile(talkJsTokenPath())
	if err != nil {
		return ""
	}
	var st struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if json.Unmarshal(data, &st) != nil || st.Token == "" {
		return ""
	}
	// 5 min buffer
	if st.ExpiresAt > 0 && time.Now().Unix() >= st.ExpiresAt-300 {
		if log != nil {
			log.Infof("TalkJS token expired (exp=%d), will refresh", st.ExpiresAt)
		}
		return ""
	}
	return strings.TrimSpace(st.Token)
}

// SaveTalkJsTokenToStorage persists token with expiry.
func SaveTalkJsTokenToStorage(token string, expiresAt int64, log *logger.Logger) error {
	path := talkJsTokenPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]any{"token": token, "expires_at": expiresAt})
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	if log != nil {
		log.Infof("TalkJS token saved to %s (exp=%d)", path, expiresAt)
	}
	return nil
}

// InvalidateTalkJsTokenStorage removes stored token (on 401).
func InvalidateTalkJsTokenStorage(log *logger.Logger) {
	_ = os.Remove(talkJsTokenPath())
	if log != nil {
		log.Infof("TalkJS token invalidated (storage cleared)")
	}
}

// TryGetTalkJsToken attempts to fetch TalkJS JWT from Eldorado API (with our cookies).
func (c *Client) TryGetTalkJsToken(ctx context.Context) (string, error) {
	return c.tryGetTalkJsTokenFromPaths(ctx, nil)
}

// TryGetTalkJsTokenForRequest tries request-scoped endpoints first, then global.
func (c *Client) TryGetTalkJsTokenForRequest(ctx context.Context, boostingRequestID string) (string, error) {
	paths := []string{
		"/api/boostingOffers/boostingRequests/" + boostingRequestID + "/talkJsToken",
		"/api/boostingOffers/boostingRequests/" + boostingRequestID + "/talkJsAuthorize",
		"/api/boostingOffers/boostingRequests/" + boostingRequestID + "/authorize",
	}
	if t, err := c.tryGetTalkJsTokenFromPaths(ctx, paths); err == nil && t != "" {
		return t, nil
	}
	return c.TryGetTalkJsToken(ctx)
}

func (c *Client) tryGetTalkJsTokenFromPaths(ctx context.Context, extraPaths []string) (string, error) {
	endpoints := []string{
		"/api/conversations/me/authorize",
		"/api/users/me/talkJsToken",
		"/api/chat/talkJsToken",
		"/api/talkjs/token",
		"/api/boostingOffers/me/talkJsToken",
	}
	if len(extraPaths) > 0 {
		endpoints = append(extraPaths, endpoints...)
	}

	for _, path := range endpoints {
		fullURL := strings.TrimRight(c.baseURL, "/") + path
		body, err := c.curlRequest(ctx, "GET", fullURL, nil)
		if err != nil {
			continue
		}
		if len(body) == 0 || body[0] == '<' {
			continue
		}

		var out struct {
			Token       string `json:"token"`
			JWT         string `json:"jwt"`
			Jwt         string `json:"Jwt"`
			AccessToken string `json:"accessToken"`
			TalkJsToken string `json:"talkJsToken"`
			Data        struct {
				Token string `json:"token"`
				JWT   string `json:"jwt"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			continue
		}
		for _, tok := range []string{
			out.Token, out.JWT, out.Jwt, out.AccessToken, out.TalkJsToken,
			out.Data.Token, out.Data.JWT,
		} {
			if strings.TrimSpace(tok) != "" {
				return strings.TrimSpace(tok), nil
			}
		}
		// Fallback: extract JWT from quoted string in JSON (e.g. "token":"eyJ...")
		s := string(body)
		if idx := strings.Index(s, "eyJ"); idx >= 0 {
			start := idx
			end := strings.IndexAny(s[start:], `"}\n\r,`)
			if end < 0 {
				end = len(s) - start
			}
			tok := s[start : start+end]
			if len(tok) > 100 && strings.Count(tok, ".") == 2 {
				return tok, nil
			}
		}
	}

	return "", fmt.Errorf("no TalkJS token endpoint found")
}

// JwtExp parses JWT and returns exp claim (0 if invalid).
func JwtExp(token string) int64 {
	parts := strings.SplitN(strings.TrimSpace(token), ".", 3)
	if len(parts) != 3 {
		return 0
	}
	// base64url decode payload
	b := parts[1]
	b = strings.ReplaceAll(b, "-", "+")
	b = strings.ReplaceAll(b, "_", "/")
	switch len(b) % 4 {
	case 2:
		b += "=="
	case 3:
		b += "="
	}
	dec, err := base64.StdEncoding.DecodeString(b)
	if err != nil {
		return 0
	}
	var p struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(dec, &p) != nil {
		return 0
	}
	return p.Exp
}

func randomHex(n int) (string, error) {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:n], nil
}

func formatUUID(hexStr string) string {
	if len(hexStr) < 32 {
		return hexStr
	}
	return hexStr[0:8] + "-" + hexStr[8:12] + "-" + hexStr[12:16] + "-" + hexStr[16:20] + "-" + hexStr[20:32]
}

func (c *Client) execCurl(ctx context.Context, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "/usr/bin/curl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("curl failed: %w (stderr=%s)", err, stderr.String())
	}
	output := stdout.String()
	httpCode := 0
	if idx := strings.LastIndex(output, "\n__HTTP_CODE__:"); idx >= 0 {
		fmt.Sscanf(output[idx+len("\n__HTTP_CODE__:"):], "%d", &httpCode)
		output = output[:idx]
	}
	body := []byte(output)
	if httpCode >= 400 {
		snippet := string(body)
		if len(snippet) > 150 {
			snippet = snippet[:150] + "..."
		}
		return body, fmt.Errorf("HTTP %d: %s", httpCode, snippet)
	}
	return body, nil
}
