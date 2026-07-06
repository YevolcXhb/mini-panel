package dto

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/minipanel/minipanel/internal/model"
)

// WebsiteCreateRequest 建站请求 DTO
// 内嵌 model.Website 字段保证向下兼容；DB* 字段为联动建库的可选项
type WebsiteCreateRequest struct {
	model.Website
	// 联动建库
	DBCreate     bool   `json:"db_create"`
	DBInstanceID uint   `json:"db_instance_id"`
	DBName       string `json:"db_name"`
	DBUsername   string `json:"db_username"`
	DBPassword   string `json:"db_password"`
	// 删站级联（仅 Delete 时使用，Create 时忽略）
	CascadeDB *bool `json:"cascade_db,omitempty"`
}

// GenerateRandomPassword 生成 16 位随机密码（小写字母+数字）
func GenerateRandomPassword() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// 极端情况下回退到固定值（不应发生）
		return fmt.Sprintf("minipanel_%d", 1234567890)
	}
	return hex.EncodeToString(b)
}
