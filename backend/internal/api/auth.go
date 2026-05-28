package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/global"
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

	// Init admin on first login
	_ = a.service.InitAdmin("admin", "admin123")

	token, err := a.service.Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, dto.Response{Code: 401, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 200, Data: dto.LoginResponse{Token: token, Username: req.Username}})
}

func (a *AuthAPI) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, dto.Response{Code: 200, Message: "ok"})
}
