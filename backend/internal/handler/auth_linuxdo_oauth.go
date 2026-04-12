package handler

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
)

const (
	linuxDoOAuthCookiePath        = "/api/v1/oauth/linuxdo"
	linuxDoOAuthStateCookieName   = "linuxdo_oauth_state"
	linuxDoOAuthVerifierCookie    = "linuxdo_oauth_verifier"
	linuxDoOAuthRedirectCookie    = "linuxdo_oauth_redirect"
	linuxDoOAuthCookieMaxAgeSec   = 10 * 60 // 10 minutes
	linuxDoOAuthDefaultRedirectTo = "/dashboard"
	linuxDoOAuthDefaultFrontendCB = "/auth/linuxdo/callback"

	linuxDoOAuthMaxRedirectLen      = 2048
	linuxDoOAuthMaxFragmentValueLen = 512
	linuxDoOAuthMaxSubjectLen       = 64 - len("linuxdo-")
)

type linuxDoTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

type linuxDoTokenExchangeError struct {
	StatusCode          int
	ProviderError       string
	ProviderDescription string
	Body                string
}

func (e *linuxDoTokenExchangeError) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("token exchange status=%d", e.StatusCode)}
	if strings.TrimSpace(e.ProviderError) != "" {
		parts = append(parts, "error="+strings.TrimSpace(e.ProviderError))
	}
	if strings.TrimSpace(e.ProviderDescription) != "" {
		parts = append(parts, "error_description="+strings.TrimSpace(e.ProviderDescription))
	}
	return strings.Join(parts, " ")
}

func (h *AuthHandler) getLinuxDoOAuthConfig(ctx context.Context) (config.LinuxDoConnectConfig, error) {
	if h != nil && h.settingSvc != nil {
		return h.settingSvc.GetLinuxDoConnectOAuthConfig(ctx)
	}
	if h == nil || h.cfg == nil {
		return config.LinuxDoConnectConfig{}, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
	}
	if !h.cfg.LinuxDo.Enabled {
		return config.LinuxDoConnectConfig{}, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
	}
	return h.cfg.LinuxDo, nil
}

func linuxDoExchangeCode(
	ctx context.Context,
	cfg config.LinuxDoConnectConfig,
	code string,
	redirectURI string,
	codeVerifier string,
) (*linuxDoTokenResponse, error) {
	client := req.C().SetTimeout(30 * time.Second)

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.ClientID)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if cfg.UsePKCE {
		form.Set("code_verifier", codeVerifier)
	}

	r := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json")

	switch strings.ToLower(strings.TrimSpace(cfg.TokenAuthMethod)) {
	case "", "client_secret_post":
		form.Set("client_secret", cfg.ClientSecret)
	case "client_secret_basic":
		r.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)
	case "none":
	default:
		return nil, fmt.Errorf("unsupported token_auth_method: %s", cfg.TokenAuthMethod)
	}

	resp, err := r.SetFormDataFromValues(form).Post(cfg.TokenURL)
	if err != nil {
		return nil, fmt.Errorf("request token: %w", err)
	}
	body := strings.TrimSpace(resp.String())
	if !resp.IsSuccessState() {
		providerErr, providerDesc := parseOAuthProviderError(body)
		return nil, &linuxDoTokenExchangeError{
			StatusCode:          resp.StatusCode,
			ProviderError:       providerErr,
			ProviderDescription: providerDesc,
			Body:                body,
		}
	}

	tokenResp, ok := parseLinuxDoTokenResponse(body)
	if !ok || strings.TrimSpace(tokenResp.AccessToken) == "" {
		return nil, &linuxDoTokenExchangeError{
			StatusCode: resp.StatusCode,
			Body:       body,
		}
	}
	if strings.TrimSpace(tokenResp.TokenType) == "" {
		tokenResp.TokenType = "Bearer"
	}
	return tokenResp, nil
}

func linuxDoFetchUserInfo(
	ctx context.Context,
	cfg config.LinuxDoConnectConfig,
	token *linuxDoTokenResponse,
) (email string, username string, subject string, err error) {
	client := req.C().SetTimeout(30 * time.Second)
	authorization, err := buildBearerAuthorization(token.TokenType, token.AccessToken)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid token for userinfo request: %w", err)
	}

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		SetHeader("Authorization", authorization).
		Get(cfg.UserInfoURL)
	if err != nil {
		return "", "", "", fmt.Errorf("request userinfo: %w", err)
	}
	if !resp.IsSuccessState() {
		return "", "", "", fmt.Errorf("userinfo status=%d", resp.StatusCode)
	}

	return linuxDoParseUserInfo(resp.String(), cfg)
}

func linuxDoParseUserInfo(body string, cfg config.LinuxDoConnectConfig) (email string, username string, subject string, err error) {
	email = firstNonEmpty(
		getGJSON(body, cfg.UserInfoEmailPath),
		getGJSON(body, "email"),
		getGJSON(body, "user.email"),
		getGJSON(body, "data.email"),
		getGJSON(body, "attributes.email"),
	)
	username = firstNonEmpty(
		getGJSON(body, cfg.UserInfoUsernamePath),
		getGJSON(body, "username"),
		getGJSON(body, "preferred_username"),
		getGJSON(body, "name"),
		getGJSON(body, "user.username"),
		getGJSON(body, "user.name"),
	)
	subject = firstNonEmpty(
		getGJSON(body, cfg.UserInfoIDPath),
		getGJSON(body, "sub"),
		getGJSON(body, "id"),
		getGJSON(body, "user_id"),
		getGJSON(body, "uid"),
		getGJSON(body, "user.id"),
	)

	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "", "", "", errors.New("userinfo missing id field")
	}
	if !isSafeLinuxDoSubject(subject) {
		return "", "", "", errors.New("userinfo returned invalid id field")
	}

	email = strings.TrimSpace(email)
	if email == "" {
		// LinuxDo Connect 的 userinfo 可能不提供 email。为兼容现有用户模型（email 必填且唯一），使用稳定的合成邮箱。
		email = linuxDoSyntheticEmail(subject)
	}

	username = strings.TrimSpace(username)
	if username == "" {
		username = "linuxdo_" + subject
	}

	return email, username, subject, nil
}

func buildLinuxDoAuthorizeURL(cfg config.LinuxDoConnectConfig, state string, codeChallenge string, redirectURI string) (string, error) {
	u, err := url.Parse(cfg.AuthorizeURL)
	if err != nil {
		return "", fmt.Errorf("parse authorize_url: %w", err)
	}

	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	if strings.TrimSpace(cfg.Scopes) != "" {
		q.Set("scope", cfg.Scopes)
	}
	q.Set("state", state)
	if cfg.UsePKCE {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

func redirectOAuthError(c *gin.Context, frontendCallback string, code string, message string, description string) {
	fragment := url.Values{}
	fragment.Set("error", truncateFragmentValue(code))
	if strings.TrimSpace(message) != "" {
		fragment.Set("error_message", truncateFragmentValue(message))
	}
	if strings.TrimSpace(description) != "" {
		fragment.Set("error_description", truncateFragmentValue(description))
	}
	redirectWithFragment(c, frontendCallback, fragment)
}

func redirectWithFragment(c *gin.Context, frontendCallback string, fragment url.Values) {
	u, err := url.Parse(frontendCallback)
	if err != nil {
		// 兜底：尽力跳转到默认页面，避免卡死在回调页。
		c.Redirect(http.StatusFound, linuxDoOAuthDefaultRedirectTo)
		return
	}
	if u.Scheme != "" && !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		c.Redirect(http.StatusFound, linuxDoOAuthDefaultRedirectTo)
		return
	}
	u.Fragment = fragment.Encode()
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.Redirect(http.StatusFound, u.String())
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func parseOAuthProviderError(body string) (providerErr string, providerDesc string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ""
	}

	providerErr = firstNonEmpty(
		getGJSON(body, "error"),
		getGJSON(body, "code"),
		getGJSON(body, "error.code"),
	)
	providerDesc = firstNonEmpty(
		getGJSON(body, "error_description"),
		getGJSON(body, "error.message"),
		getGJSON(body, "message"),
		getGJSON(body, "detail"),
	)

	if providerErr != "" || providerDesc != "" {
		return providerErr, providerDesc
	}

	values, err := url.ParseQuery(body)
	if err != nil {
		return "", ""
	}
	providerErr = firstNonEmpty(values.Get("error"), values.Get("code"))
	providerDesc = firstNonEmpty(values.Get("error_description"), values.Get("error_message"), values.Get("message"))
	return providerErr, providerDesc
}

func parseLinuxDoTokenResponse(body string) (*linuxDoTokenResponse, bool) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, false
	}

	accessToken := strings.TrimSpace(getGJSON(body, "access_token"))
	if accessToken != "" {
		tokenType := strings.TrimSpace(getGJSON(body, "token_type"))
		refreshToken := strings.TrimSpace(getGJSON(body, "refresh_token"))
		scope := strings.TrimSpace(getGJSON(body, "scope"))
		expiresIn := gjson.Get(body, "expires_in").Int()
		return &linuxDoTokenResponse{
			AccessToken:  accessToken,
			TokenType:    tokenType,
			ExpiresIn:    expiresIn,
			RefreshToken: refreshToken,
			Scope:        scope,
		}, true
	}

	values, err := url.ParseQuery(body)
	if err != nil {
		return nil, false
	}
	accessToken = strings.TrimSpace(values.Get("access_token"))
	if accessToken == "" {
		return nil, false
	}
	expiresIn := int64(0)
	if raw := strings.TrimSpace(values.Get("expires_in")); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			expiresIn = v
		}
	}
	return &linuxDoTokenResponse{
		AccessToken:  accessToken,
		TokenType:    strings.TrimSpace(values.Get("token_type")),
		ExpiresIn:    expiresIn,
		RefreshToken: strings.TrimSpace(values.Get("refresh_token")),
		Scope:        strings.TrimSpace(values.Get("scope")),
	}, true
}

func getGJSON(body string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	res := gjson.Get(body, path)
	if !res.Exists() {
		return ""
	}
	return res.String()
}

func truncateLogValue(value string, maxLen int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxLen <= 0 {
		return ""
	}
	if len(value) <= maxLen {
		return value
	}
	value = value[:maxLen]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func singleLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

func sanitizeFrontendRedirectPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if len(path) > linuxDoOAuthMaxRedirectLen {
		return ""
	}
	// 只允许同源相对路径（避免开放重定向）。
	if !strings.HasPrefix(path, "/") {
		return ""
	}
	if strings.HasPrefix(path, "//") {
		return ""
	}
	if strings.Contains(path, "://") {
		return ""
	}
	if strings.ContainsAny(path, "\r\n") {
		return ""
	}
	return path
}

func isRequestHTTPS(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	proto := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")))
	return proto == "https"
}

func encodeCookieValue(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeCookieValue(value string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func readCookieDecoded(c *gin.Context, name string) (string, error) {
	ck, err := c.Request.Cookie(name)
	if err != nil {
		return "", err
	}
	return decodeCookieValue(ck.Value)
}

func setCookie(c *gin.Context, name string, value string, maxAgeSec int, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     linuxDoOAuthCookiePath,
		MaxAge:   maxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearCookie(c *gin.Context, name string, secure bool) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     linuxDoOAuthCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func truncateFragmentValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) > linuxDoOAuthMaxFragmentValueLen {
		value = value[:linuxDoOAuthMaxFragmentValueLen]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	return value
}

func buildBearerAuthorization(tokenType, accessToken string) (string, error) {
	tokenType = strings.TrimSpace(tokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}
	if !strings.EqualFold(tokenType, "Bearer") {
		return "", fmt.Errorf("unsupported token_type: %s", tokenType)
	}

	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return "", errors.New("missing access_token")
	}
	if strings.ContainsAny(accessToken, " \t\r\n") {
		return "", errors.New("access_token contains whitespace")
	}
	return "Bearer " + accessToken, nil
}

func isSafeLinuxDoSubject(subject string) bool {
	subject = strings.TrimSpace(subject)
	if subject == "" || len(subject) > linuxDoOAuthMaxSubjectLen {
		return false
	}
	for _, r := range subject {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func linuxDoSyntheticEmail(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
	}
	return "linuxdo-" + subject + service.LinuxDoConnectSyntheticEmailDomain
}
