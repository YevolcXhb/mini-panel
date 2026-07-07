package tools

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// ToolCache 工具结果缓存接口
type ToolCache interface {
	Get(key string) (string, bool)
	Set(key, value string, ttl time.Duration)
	Delete(key string)
}

// CacheableTool 可选接口：工具可选择实现以启用结果缓存。
// 查询类工具应实现并返回 true；写操作工具不应实现或返回 false。
type CacheableTool interface {
	Cacheable() bool
}

// CacheKey 根据工具名和参数生成缓存键（SHA256 前16位）
func CacheKey(toolName, args string) string {
	h := sha256.Sum256([]byte(toolName + "|" + args))
	return hex.EncodeToString(h[:])[:16]
}

// cacheEntry 缓存条目
type cacheEntry struct {
	key       string
	value     string
	expiresAt time.Time
}

// MemoryLRUCache 进程内 LRU 缓存（带 TTL 过期）
type MemoryLRUCache struct {
	mu       sync.Mutex
	capacity int
	maxSize  int // 单项最大字节数
	items    map[string]*list.Element
	order    *list.List // front = 最近使用
}

// NewMemoryLRUCache 创建 LRU 缓存
// capacity: 最大项数；maxSize: 单项最大字节数（0 表示不限制）
func NewMemoryLRUCache(capacity, maxSize int) *MemoryLRUCache {
	if capacity <= 0 {
		capacity = 256
	}
	return &MemoryLRUCache{
		capacity: capacity,
		maxSize:  maxSize,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Get 查询缓存
func (c *MemoryLRUCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[key]
	if !ok {
		return "", false
	}
	entry := elem.Value.(*cacheEntry)
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		c.removeElement(elem)
		return "", false
	}
	c.order.MoveToFront(elem)
	return entry.value, true
}

// Set 写入缓存
func (c *MemoryLRUCache) Set(key, value string, ttl time.Duration) {
	if c.maxSize > 0 && len(value) > c.maxSize {
		return // 超过单项大小限制，不缓存
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*cacheEntry)
		entry.value = value
		if ttl > 0 {
			entry.expiresAt = time.Now().Add(ttl)
		}
		c.order.MoveToFront(elem)
		return
	}
	entry := &cacheEntry{key: key, value: value}
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.removeElement(oldest)
		}
	}
}

// Delete 删除单个缓存项
func (c *MemoryLRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
	}
}

func (c *MemoryLRUCache) removeElement(elem *list.Element) {
	entry := elem.Value.(*cacheEntry)
	delete(c.items, entry.key)
	c.order.Remove(elem)
}

// 全局工具缓存实例
var (
	globalToolCache     ToolCache
	globalToolCacheOnce sync.Once
)

// GetGlobalToolCache 获取进程级默认工具缓存
func GetGlobalToolCache() ToolCache {
	globalToolCacheOnce.Do(func() {
		globalToolCache = NewMemoryLRUCache(256, 1024*1024) // 256项，单项最大1MB
	})
	return globalToolCache
}

// SetGlobalToolCache 替换全局缓存（用于注入自定义实现）
func SetGlobalToolCache(cache ToolCache) {
	globalToolCache = cache
}
