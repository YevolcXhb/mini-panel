package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/service"
)

type AuthAPI struct {
	service *service.AuthService
}

func NewAuthAPI() *AuthAPI {
	return &AuthAPI{service: service.NewAuthService()}
}

func (a *AuthAPI) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}

	_ = a.service.InitAdmin("admin", "admin123")

	token, err := a.service.Login(req.Username, req.Password, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Response{Code: 401, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: dto.LoginResponse{Token: token, Username: req.Username}})
}

func (a *AuthAPI) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "ok"})
}

func (a *AuthAPI) ChangePassword(c *gin.Context) {
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	user, _ := c.Get("user")
	username, ok := user.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, dto.Response{Code: 401, Message: "unauthorized"})
		return
	}
	if err := a.service.ChangePassword(username, req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, dto.Response{Code: 400, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "password changed"})
}
