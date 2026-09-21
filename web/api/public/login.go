package public

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/accounts"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/utils"
	"github.com/komari-monitor/komari/web/api"

	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TwoFa    string `json:"2fa_code"`
}

const sessionCookieMaxAge = 2592000

const (
	loginWindow = 10 * time.Minute
	loginMaxFailures = 8
)

type loginAttempt struct {
	Failures int
	ResetAt time.Time
}

var loginAttempts = struct {
	sync.Mutex
	items map[string]loginAttempt
}{items: make(map[string]loginAttempt)}

func loginRateLimited(ip string) bool {
	now := time.Now()
	loginAttempts.Lock()
	defer loginAttempts.Unlock()
	a := loginAttempts.items[ip]
	if a.ResetAt.Before(now) {
		delete(loginAttempts.items, ip)
		return false
	}
	return a.Failures >= loginMaxFailures
}

func recordLoginFailure(ip string) {
	now := time.Now()
	loginAttempts.Lock()
	defer loginAttempts.Unlock()
	a := loginAttempts.items[ip]
	if a.ResetAt.Before(now) {
		a = loginAttempt{ResetAt: now.Add(loginWindow)}
	}
	a.Failures++
	loginAttempts.items[ip] = a
}

func clearLoginFailures(ip string) {
	loginAttempts.Lock()
	delete(loginAttempts.items, ip)
	loginAttempts.Unlock()
}

func setSessionCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "session_token",
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Secure:   utils.GetScheme(c) == "https",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func Login(c *gin.Context) {
	clientIP := c.ClientIP()
	if loginRateLimited(clientIP) {
		c.Header("Retry-After", "600")
		api.RespondError(c, http.StatusTooManyRequests, "Too many failed login attempts; try again later")
		return
	}
	DisablePasswordLogin, _ := config.GetAs[bool](config.DisablePasswordLoginKey, false)
	if DisablePasswordLogin {
		api.RespondError(c, http.StatusForbidden, "Password login is disabled")
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	var data LoginRequest
	err = json.Unmarshal(bodyBytes, &data)
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if data.Username == "" || data.Password == "" {
		api.RespondError(c, http.StatusBadRequest, "Invalid request body: Username and password are required")
		return
	}

	uuid, success := accounts.CheckPassword(data.Username, data.Password)
	if !success {
		recordLoginFailure(clientIP)
		api.RespondError(c, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	// 2FA
	user, _ := accounts.GetUserByUUID(uuid)
	if user.TwoFactor != "" { // 开启了2FA
		if data.TwoFa == "" {
			recordLoginFailure(clientIP)
			api.RespondError(c, http.StatusUnauthorized, "2FA code is required")
			return
		}
		if ok, err := accounts.Verify2Fa(uuid, data.TwoFa); err != nil || !ok {
			recordLoginFailure(clientIP)
			api.RespondError(c, http.StatusUnauthorized, "Invalid 2FA code")
			return
		}
	}
	// Create session
	session, err := accounts.CreateSession(uuid, sessionCookieMaxAge, c.Request.UserAgent(), c.ClientIP(), "password")
	if err != nil {
		api.RespondError(c, http.StatusInternalServerError, "Failed to create session: "+err.Error())
		return
	}
	setSessionCookie(c, session, sessionCookieMaxAge)
	clearLoginFailures(clientIP)
	auditlog.Log(c.ClientIP(), uuid, "logged in (password)", "login")
	api.RespondSuccess(c, gin.H{"logged_in": true})
}
func Logout(c *gin.Context) {
	session, _ := c.Cookie("session_token")
	accounts.DeleteSession(session)
	setSessionCookie(c, "", -1)
	auditlog.Log(c.ClientIP(), "", "logged out", "logout")
	c.Redirect(302, "/")
}
