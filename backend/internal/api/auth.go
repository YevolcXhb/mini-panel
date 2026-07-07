package api

import (
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
	"github.com/minipanel/minipanel/internal/utils/captcha"
)

type AuthAPI struct {
	service *service.AuthService
}

func NewAuthAPI() *AuthAPI {
	return &AuthAPI{service: service.NewAuthService()}
}

type ipTracker struct {
	mu         sync.Mutex
	failures   map[string]int
	lockUntil  map[string]time.Time
	lastFail   map[string]time.Time
}

var ipTrack = &ipTracker{
	failures:  make(map[string]int),
	lockUntil: make(map[string]time.Time),
	lastFail:  make(map[string]time.Time),
}

func (t *ipTracker) isLocked(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if until, ok := t.lockUntil[ip]; ok {
		if time.Now().Before(until) {
			return true
		}
		delete(t.lockUntil, ip)
		delete(t.failures, ip)
	}
	return false
}

func (t *ipTracker) needCaptcha(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failures[ip] >= 2 {
		return true
	}
	return false
}

func (t *ipTracker) recordFailure(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures[ip]++
	t.lastFail[ip] = time.Now()
	if t.failures[ip] >= 5 {
		t.lockUntil[ip] = time.Now().Add(15 * time.Minute)
	}
}

func (t *ipTracker) recordSuccess(ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, ip)
	delete(t.lockUntil, ip)
	delete(t.lastFail, ip)
}

func (a *AuthAPI) Captcha(c *gin.Context) {
	id, code := captcha.Generate()
	imgBytes, err := captcha.GenerateImage(code)
	if err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 500, Message: "生成验证码失败"})
		return
	}
	c.JSON(http.StatusOK, dto.Response{
		Code: 200,
		Data: gin.H{
			"captcha_id": id,
			"image":      base64.StdEncoding.EncodeToString(imgBytes),
		},
	})
}

func (a *AuthAPI) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 400, Message: err.Error()})
		return
	}

	ip := c.ClientIP()

	if ipTrack.isLocked(ip) {
		c.JSON(http.StatusOK, dto.Response{Code: 429, Message: "登录尝试过多，请15分钟后再试"})
		return
	}

	if ipTrack.needCaptcha(ip) {
		if req.Captcha == "" || req.CaptchaID == "" {
			c.JSON(http.StatusOK, dto.Response{Code: 400, Message: "需要验证码"})
			return
		}
		if !captcha.Verify(req.CaptchaID, req.Captcha) {
			ipTrack.recordFailure(ip)
			c.JSON(http.StatusOK, dto.Response{Code: 400, Message: "验证码错误"})
			return
		}
	}

	_ = a.service.InitAdmin("admin", "admin123")

	token, err := a.service.Login(req.Username, req.Password, ip)
	if err != nil {
		ipTrack.recordFailure(ip)
		c.JSON(http.StatusUnauthorized, dto.Response{Code: 401, Message: err.Error()})
		return
	}

	ipTrack.recordSuccess(ip)
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: dto.LoginResponse{Token: token, Username: req.Username}})
}

func (a *AuthAPI) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "ok"})
}

func (a *AuthAPI) ChangePassword(c *gin.Context) {
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	user, _ := c.Get("user")
	username, ok := user.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.Response{Code: 401, Message: "unauthorized"})
		return
	}
	if err := a.service.ChangePassword(username, req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusOK, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "password changed"})
}