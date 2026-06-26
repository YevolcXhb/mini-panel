package captcha

import (
	"crypto/rand"
	"encoding/base64"
	"math/big"
	"sync"
	"time"
)

type Store struct {
	mu   sync.Mutex
	data map[string]storeItem
}

type storeItem struct {
	code     string
	expireAt time.Time
}

var defaultStore = &Store{data: make(map[string]storeItem)}

func init() {
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			defaultStore.clean()
		}
	}()
}

func (s *Store) clean() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.data {
		if now.After(v.expireAt) {
			delete(s.data, k)
		}
	}
}

func (s *Store) Set(id, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = storeItem{code: code, expireAt: time.Now().Add(5 * time.Minute)}
}

func (s *Store) Verify(id, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.data[id]
	if !ok {
		return false
	}
	delete(s.data, id)
	if time.Now().After(item.expireAt) {
		return false
	}
	return item.code == code
}

func NewID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func GenerateCode() string {
	code := ""
	for i := 0; i < 4; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		code += n.String()
	}
	return code
}

func Generate() (id string, code string) {
	id = NewID()
	code = GenerateCode()
	defaultStore.Set(id, code)
	return
}

func Verify(id, code string) bool {
	return defaultStore.Verify(id, code)
}