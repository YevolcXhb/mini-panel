package tools

import (
	"time"

	"github.com/minipanel/minipanel/internal/agent/compression"
	"github.com/minipanel/minipanel/internal/global"
	"gorm.io/gorm"
)

// AgentLazyRef SQLite 持久化的 lazy-ref 记录
type AgentLazyRef struct {
	Hash      string    `gorm:"primaryKey;column:hash"`
	Content   string    `gorm:"column:content;type:text"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
}

func (AgentLazyRef) TableName() string { return "agent_lazy_refs" }

// PersistentLazyRefStore SQLite 持久化的 lazy-ref 存储
type PersistentLazyRefStore struct {
	defaultTTL time.Duration
}

// NewPersistentLazyRefStore 创建持久化存储并自动迁移表结构
func NewPersistentLazyRefStore(defaultTTL time.Duration) *PersistentLazyRefStore {
	if defaultTTL <= 0 {
		defaultTTL = 24 * time.Hour
	}
	store := &PersistentLazyRefStore{defaultTTL: defaultTTL}
	store.autoMigrate()
	go store.cleanupLoop()
	return store
}

func (s *PersistentLazyRefStore) autoMigrate() {
	if global.DB == nil {
		return
	}
	if err := global.DB.AutoMigrate(&AgentLazyRef{}); err != nil {
		global.LOG.Errorf("[LazyRef] 自动迁移失败: %v", err)
	}
}

func (s *PersistentLazyRefStore) Register(content string) string {
	hash := compression.HashContent(content)
	if global.DB == nil {
		return hash
	}
	expiresAt := time.Now().Add(s.defaultTTL)
	record := AgentLazyRef{
		Hash:      hash,
		Content:   content,
		ExpiresAt: expiresAt,
	}
	// 使用 OnConflict 进行 upsert
	if err := global.DB.Where("hash = ?", hash).
		Assign(AgentLazyRef{Content: content, ExpiresAt: expiresAt}).
		FirstOrCreate(&record).Error; err != nil {
		global.LOG.Warnf("[LazyRef] Register 写入失败: %v", err)
	}
	return hash
}

func (s *PersistentLazyRefStore) Resolve(hash string) (string, bool) {
	if global.DB == nil {
		return "", false
	}
	var record AgentLazyRef
	result := global.DB.Where("hash = ?", hash).First(&record)
	if result.Error != nil {
		if result.Error != gorm.ErrRecordNotFound {
			global.LOG.Warnf("[LazyRef] Resolve 查询失败: %v", result.Error)
		}
		return "", false
	}
	if !record.ExpiresAt.IsZero() && time.Now().After(record.ExpiresAt) {
		global.DB.Where("hash = ?", hash).Delete(&AgentLazyRef{})
		return "", false
	}
	return record.Content, true
}

func (s *PersistentLazyRefStore) Delete(hash string) {
	if global.DB == nil {
		return
	}
	global.DB.Where("hash = ?", hash).Delete(&AgentLazyRef{})
}

// cleanupLoop 后台定期清理过期记录
func (s *PersistentLazyRefStore) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if global.DB == nil {
			continue
		}
		result := global.DB.Where("expires_at < ?", time.Now()).Delete(&AgentLazyRef{})
		if result.RowsAffected > 0 {
			global.LOG.Infof("[LazyRef] 清理过期记录 %d 条", result.RowsAffected)
		}
	}
}

// InitPersistentLazyRefStore 初始化 SQLite 持久化 lazy-ref 存储并注入为全局存储。
// 应在应用启动时调用（main.go 或初始化函数）。
func InitPersistentLazyRefStore() {
	store := NewPersistentLazyRefStore(24 * time.Hour)
	compression.SetGlobalLazyRefStore(store)
	global.LOG.Infof("[LazyRef] 持久化存储已初始化，TTL=24h")
}
