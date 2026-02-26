package eldorado

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"eldorado-bot/internal/logger"
)

// CookieAuth manages cookie-based authentication for the Eldorado API.
// Eldorado uses AWS Cognito with server-side token refresh, so we just need
// to provide the session cookies and the server handles refreshing internally.
type CookieAuth struct {
	log *logger.Logger
}

func NewCookieAuth(log *logger.Logger) *CookieAuth {
	return &CookieAuth{log: log}
}

// BuildJar creates a cookie jar from the raw cookie string (from browser DevTools).
func (a *CookieAuth) BuildJar(rawCookies string) (http.CookieJar, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse("https://www.eldorado.gg")

	var cookies []*http.Cookie
	for _, part := range strings.Split(rawCookies, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eqIdx := strings.Index(part, "=")
		if eqIdx < 0 {
			continue
		}
		name := strings.TrimSpace(part[:eqIdx])
		value := strings.TrimSpace(part[eqIdx+1:])
		cookies = append(cookies, &http.Cookie{
			Name:  name,
			Value: value,
		})
	}

	if len(cookies) == 0 {
		return nil, fmt.Errorf("no cookies parsed from ELDORADO_COOKIES")
	}

	jar.SetCookies(u, cookies)

	hasIdToken := false
	for _, c := range cookies {
		if strings.Contains(c.Name, "EldoradoIdToken") {
			hasIdToken = true
			break
		}
	}
	if !hasIdToken {
		a.log.Errorf("warning: no EldoradoIdToken found in cookies; API calls may fail")
	}

	a.log.Infof("auth: loaded %d cookies", len(cookies))
	return jar, nil
}
