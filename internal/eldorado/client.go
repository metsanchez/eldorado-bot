package eldorado

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"sync"

	"eldorado-bot/internal/logger"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// ErrAuthExpired is returned when the API responds with 401 (cookies expired).
var ErrAuthExpired = fmt.Errorf("authentication expired")

type Client struct {
	baseURL    string
	log        *logger.Logger
	rawCookies string
	xsrfToken  string

	email    string
	password string

	mu       sync.Mutex
	loginMu  sync.Mutex // prevents concurrent re-logins
	loginGen int        // incremented after each successful login
}

func NewClient(baseURL, email, password, rawCookies, xsrfToken string, log *logger.Logger) *Client {
	return &Client{
		baseURL:    baseURL,
		log:        log,
		rawCookies: rawCookies,
		xsrfToken:  xsrfToken,
		email:      email,
		password:   password,
	}
}

// Login authenticates with Eldorado. If cookies are already provided and valid,
// uses those. Otherwise launches headless Chrome to perform browser login.
func (c *Client) Login(ctx context.Context) error {
	if c.rawCookies != "" {
		c.log.Infof("eldorado: testing existing cookies (%d chars)", len(c.rawCookies))
		if c.testCookies(ctx) {
			c.log.Infof("eldorado: existing cookies are valid")
			return nil
		}
		c.log.Infof("eldorado: existing cookies expired, will re-login via browser")
	}

	return c.browserLogin(ctx)
}

func (c *Client) browserLogin(ctx context.Context) error {
	if c.email == "" || c.password == "" {
		return fmt.Errorf("ELDORADO_EMAIL and ELDORADO_PASSWORD are required for auto-login")
	}

	result, err := BrowserLogin(ctx, c.baseURL, c.email, c.password, c.log)
	if err != nil {
		return fmt.Errorf("browser login failed: %w", err)
	}

	// Log cookie names for debugging
	var names []string
	for _, part := range strings.Split(result.CookieString, "; ") {
		if eqIdx := strings.Index(part, "="); eqIdx > 0 {
			names = append(names, part[:eqIdx])
		}
	}
	c.log.Infof("browser-login: cookie names: %v", names)
	c.log.Infof("browser-login: xsrf token present: %v (len=%d)", result.XSRFToken != "", len(result.XSRFToken))

	c.mu.Lock()
	c.rawCookies = result.CookieString
	c.xsrfToken = result.XSRFToken
	c.mu.Unlock()

	return nil
}

func (c *Client) testCookies(ctx context.Context) bool {
	body, err := c.curlRequestRaw(ctx, "GET", c.baseURL+"/api/boostingOffers/me/boostingSubscriptions", nil)
	if err != nil {
		return false
	}
	if len(body) == 0 {
		return false
	}
	if body[0] == '<' {
		return false
	}
	return true
}

// reloginAndRetry detects 401/auth errors and attempts a browser re-login.
// Uses loginMu to prevent multiple goroutines from launching Chrome simultaneously.
func (c *Client) reloginAndRetry(ctx context.Context, method, fullURL string, jsonBody []byte, originalErr error) ([]byte, error) {
	if c.email == "" || c.password == "" {
		return nil, originalErr
	}

	c.mu.Lock()
	genBefore := c.loginGen
	c.mu.Unlock()

	// Only one goroutine does the actual re-login; others wait and reuse new cookies
	c.loginMu.Lock()
	defer c.loginMu.Unlock()

	c.mu.Lock()
	genAfter := c.loginGen
	c.mu.Unlock()

	if genAfter > genBefore {
		// Another goroutine already re-logged in while we waited
		c.log.Infof("eldorado: re-login already done by another goroutine, retrying request")
		return c.curlRequestRaw(ctx, method, fullURL, jsonBody)
	}

	c.log.Infof("eldorado: got auth error, attempting browser re-login...")
	if err := c.browserLogin(ctx); err != nil {
		return nil, fmt.Errorf("re-login failed: %w (original error: %v)", err, originalErr)
	}

	c.mu.Lock()
	c.loginGen++
	c.mu.Unlock()

	c.log.Infof("eldorado: re-login successful, retrying request")
	return c.curlRequestRaw(ctx, method, fullURL, jsonBody)
}

func (c *Client) ListReceivedBoostingRequests(ctx context.Context, filter string, gameID string) (*BoostingRequestPage, error) {
	q := url.Values{}
	q.Set("filter", filter)
	if gameID != "" {
		q.Set("gameId", gameID)
	}
	q.Set("pageSize", "50")
	q.Set("pageDirection", "Next")
	q.Set("cursorValue", "9999-99-99 99:99:99.999999999999999-9999-9999-9999-999999999999")

	var page BoostingRequestPage
	if err := c.doGet(ctx, "/api/boostingOffers/me/boostingRequests/received", q, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (c *Client) GetBoostingRequestDetails(ctx context.Context, requestID string) (*BoostingRequestFull, error) {
	path := fmt.Sprintf("/api/boostingOffers/boostingRequests/%s", requestID)
	var detail BoostingRequestFull
	if err := c.doGet(ctx, path, nil, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

func (c *Client) CreateBoostingOffer(ctx context.Context, req BoostingOfferPost) (*BoostingOfferPublic, error) {
	var resp BoostingOfferPublic
	if err := c.doPost(ctx, "/api/boostingOffers", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CreateConversationForSeller(ctx context.Context, boostingRequestID string) (*BoostingConversation, error) {
	path := fmt.Sprintf("/api/boostingOffers/boostingRequests/%s/createConversationForSeller", boostingRequestID)
	var resp BoostingConversation
	if err := c.doPost(ctx, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListSellerOrders(ctx context.Context, orderState string) (*OrderPage, error) {
	q := url.Values{}
	if orderState != "" {
		q.Set("orderState", orderState)
	}
	q.Set("pageSize", "50")

	var page OrderPage
	if err := c.doGet(ctx, "/api/orders/me/seller/orders", q, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

func (c *Client) MarkBoostingRequestViewed(ctx context.Context, boostingRequestID string) error {
	path := fmt.Sprintf("/api/boostingOffers/boostingRequests/%s/viewer", boostingRequestID)
	_, err := c.curlRequest(ctx, "PUT", c.baseURL+path, nil)
	return err
}

func (c *Client) ListBoostingSubscriptions(ctx context.Context) ([]BoostingSubscription, error) {
	var subs []BoostingSubscription
	if err := c.doGet(ctx, "/api/boostingOffers/me/boostingSubscriptions", nil, &subs); err != nil {
		return nil, err
	}
	return subs, nil
}

// --- HTTP helpers using curl to bypass Cloudflare TLS fingerprinting ---

func (c *Client) doGet(ctx context.Context, path string, query url.Values, out any) error {
	fullURL := c.baseURL + path
	if query != nil {
		fullURL += "?" + query.Encode()
	}

	body, err := c.curlRequest(ctx, "GET", fullURL, nil)
	if err != nil {
		return err
	}

	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			snippet := string(body)
			if len(snippet) > 300 {
				snippet = snippet[:300]
			}
			return fmt.Errorf("decode response: %w (body=%s)", err, snippet)
		}
	}
	return nil
}

func (c *Client) doPost(ctx context.Context, path string, reqBody any, out any) error {
	fullURL := c.baseURL + path

	var jsonBody []byte
	if reqBody != nil {
		var err error
		jsonBody, err = json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
	}

	body, err := c.curlRequest(ctx, "POST", fullURL, jsonBody)
	if err != nil {
		return err
	}

	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			snippet := string(body)
			if len(snippet) > 300 {
				snippet = snippet[:300]
			}
			return fmt.Errorf("decode response: %w (body=%s)", err, snippet)
		}
	}
	return nil
}

// curlRequest makes a request and auto-retries with browser login on auth failure.
func (c *Client) curlRequest(ctx context.Context, method, fullURL string, jsonBody []byte) ([]byte, error) {
	body, err := c.curlRequestRaw(ctx, method, fullURL, jsonBody)
	if err != nil {
		if isAuthError(err, body) {
			retryBody, retryErr := c.reloginAndRetry(ctx, method, fullURL, jsonBody, err)
			if retryErr != nil {
				return nil, retryErr
			}
			return retryBody, nil
		}
		return nil, err
	}

	if isAuthErrorBody(body) {
		retryBody, retryErr := c.reloginAndRetry(ctx, method, fullURL, jsonBody, ErrAuthExpired)
		if retryErr != nil {
			return nil, retryErr
		}
		return retryBody, nil
	}

	return body, nil
}

// curlRequestRaw executes curl without auto-retry logic.
func (c *Client) curlRequestRaw(ctx context.Context, method, fullURL string, jsonBody []byte) ([]byte, error) {
	c.mu.Lock()
	cookies := c.rawCookies
	xsrf := c.xsrfToken
	c.mu.Unlock()

	args := []string{
		"-sL", "--compressed",
		"--max-time", "20",
		"-X", method,
		"-H", "Accept: application/json, text/plain, */*",
		"-H", "User-Agent: " + userAgent,
		"-H", "Origin: " + c.baseURL,
		"-H", "Referer: " + c.baseURL + "/",
		"-b", cookies,
		"-w", "\n__HTTP_CODE__:%{http_code}",
	}

	if xsrf != "" {
		args = append(args, "-H", "X-XSRF-Token: "+xsrf)
	}

	if jsonBody != nil {
		args = append(args, "-H", "Content-Type: application/json", "-d", string(jsonBody))
	}

	args = append(args, fullURL)

	cmd := exec.CommandContext(ctx, "/usr/bin/curl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("curl %s %s failed: %w (stderr=%s)", method, fullURL, err, stderr.String())
	}

	output := stdout.String()

	// Parse HTTP status code from the end
	httpCode := 0
	if idx := strings.LastIndex(output, "\n__HTTP_CODE__:"); idx >= 0 {
		codeStr := output[idx+len("\n__HTTP_CODE__:"):]
		fmt.Sscanf(codeStr, "%d", &httpCode)
		output = output[:idx]
	}

	body := []byte(output)

	if httpCode == 401 || httpCode == 403 {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		c.log.Infof("curl: %s ...%s returned HTTP %d body=%s", method, fullURL[len(fullURL)-min(len(fullURL), 60):], httpCode, snippet)
		return body, ErrAuthExpired
	}

	if len(body) == 0 {
		return body, nil
	}

	if body[0] == '<' || strings.Contains(string(body[:min(len(body), 100)]), "<!DOCTYPE") {
		return body, ErrAuthExpired
	}

	return body, nil
}

func isAuthError(err error, body []byte) bool {
	if err == ErrAuthExpired {
		return true
	}
	if err != nil && strings.Contains(err.Error(), "Cloudflare challenge") {
		return true
	}
	return false
}

func isAuthErrorBody(body []byte) bool {
	return false
}
