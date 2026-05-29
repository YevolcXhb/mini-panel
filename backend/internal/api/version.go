package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/minipanel/minipanel/internal/dto"
	"github.com/minipanel/minipanel/internal/global"
)

type VersionAPI struct{}

func NewVersionAPI() *VersionAPI {
	return &VersionAPI{}
}

type VersionInfo struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	GitCommit string `json:"git_commit"`
}

func (a *VersionAPI) Get(c *gin.Context) {
	c.JSON(http.StatusOK, dto.Response{
		Code: 200,
		Data: VersionInfo{
			Version:   global.Version,
			BuildTime: global.BuildTime,
			GitCommit: global.GitCommit,
		},
	})
}
