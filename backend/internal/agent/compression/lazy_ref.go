package compression

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// LazyRefStore 管理 lazy-ref 的注册与解析。
// 实现可选用内存 LRU 或 SQLite 持久化（见 tools/lazy_ref_store.go）。
type LazyRefStore interface {
	// Register 存入内容并返回引用 hash（前12位）
	Register(content string) string
	// Resolve 根据 hash 取回完整内容
	Resolve(hash string) (string, bool)
	// Delete 清除单个引用
	Delete(hash string)
}

// lazyEntry 存储条目
type lazyEntry struct {
	content   string
	expiresAt time.Time
}

// MemoryLazyRefStore 进程内 lazy-ref 存储（带 TTL 过期）
type MemoryLazyRefStore struct {
	mu      sync.RWMutex
	entries map[string]lazyEntry
	ttl     time.Duration
}

// NewMemoryLazyRefStore 创建内存存储，ttl=0 表示永不过期
func NewMemoryLazyRefStore(ttl time.Duration) *MemoryLazyRefStore {
	store := &MemoryLazyRefStore{
		entries: make(map[string]lazyEntry),
		ttl:     ttl,
	}
	if ttl > 0 {
		go store.cleanupLoop()
	}
	return store
}

// HashContent 计算 SHA256 前12位作为引用（导出供外部实现使用）
func HashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])[:12]
}

// hashContent 是 HashContent 的别名（内部使用）
func hashContent(content string) string {
	return HashContent(content)
}

func (s *MemoryLazyRefStore) Register(content string) string {
	hash := hashContent(content)
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry := time.Time{}
	if s.ttl > 0 {
		expiry = time.Now().Add(s.ttl)
	}
	s.entries[hash] = lazyEntry{content: content, expiresAt: expiry}
	return hash
}

func (s *MemoryLazyRefStore) Resolve(hash string) (string, bool) {
	s.mu.RLock()
	entry, ok := s.entries[hash]
	s.mu.RUnlock()
	if !ok {
		return "", false
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		s.mu.Lock()
		delete(s.entries, hash)
		s.mu.Unlock()
		return "", false
	}
	return entry.content, true
}

func (s *MemoryLazyRefStore) Delete(hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, hash)
}

// cleanupLoop 后台定期清理过期项
func (s *MemoryLazyRefStore) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for k, v := range s.entries {
			if !v.expiresAt.IsZero() && now.After(v.expiresAt) {
				delete(s.entries, k)
			}
		}
		s.mu.Unlock()
	}
}

// globalLazyRefStore 进程级默认存储（内存，TTL 1 小时）
var (
	globalLazyRefStore     LazyRefStore
	globalLazyRefStoreOnce sync.Once
)

// GetGlobalLazyRefStore 获取进程级默认 lazy-ref 存储
func GetGlobalLazyRefStore() LazyRefStore {
	globalLazyRefStoreOnce.Do(func() {
		globalLazyRefStore = NewMemoryLazyRefStore(1 * time.Hour)
	})
	return globalLazyRefStore
}

// SetGlobalLazyRefStore 替换全局存储（用于注入 SQLite 持久化版本）
func SetGlobalLazyRefStore(store LazyRefStore) {
	globalLazyRefStore = store
}

// RegisterLazyRef 使用全局存储注册 lazy-ref
func RegisterLazyRef(content string) string {
	return GetGlobalLazyRefStore().Register(content)
}

// ResolveLazyRef 使用全局存储解析 lazy-ref
func ResolveLazyRef(hash string) (string, bool) {
	return GetGlobalLazyRefStore().Resolve(hash)
}
