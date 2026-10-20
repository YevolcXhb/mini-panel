package agentcore

import (
	"sync"
	"testing"
)

func TestInboxPushDrain(t *testing.T) {
	q := NewInbox()
	if !q.Push("第一条") {
		t.Fatal("非空消息应入队成功")
	}
	if !q.Push("  第二条  ") {
		t.Fatal("带空白的消息应入队成功并 trim")
	}
	if q.Pending() != 2 {
		t.Fatalf("积压应为 2，实际 %d", q.Pending())
	}
	got := q.Drain()
	if len(got) != 2 {
		t.Fatalf("应取出 2 条，实际 %d", len(got))
	}
	if got[0] != "第一条" || got[1] != "第二条" {
		t.Fatalf("内容或 trim 不正确: %#v", got)
	}
	if q.Pending() != 0 {
		t.Fatalf("取走后应为 0，实际 %d", q.Pending())
	}
	if again := q.Drain(); again != nil {
		t.Fatalf("二次取走应返回 nil，实际 %#v", again)
	}
}

func TestInboxRejectsBlank(t *testing.T) {
	q := NewInbox()
	for _, s := range []string{"", "   ", "\n\t ", "\r\n"} {
		if q.Push(s) {
			t.Fatalf("空白消息 %q 不应入队", s)
		}
	}
	if q.Pending() != 0 {
		t.Fatalf("空白消息不应占用队列，实际 %d", q.Pending())
	}
}

func TestInboxTotalAndClear(t *testing.T) {
	q := NewInbox()
	q.Push("a")
	q.Push("b")
	q.Drain()
	q.Push("c")
	if q.Total() != 3 {
		t.Fatalf("累计计数应为 3，实际 %d", q.Total())
	}
	q.Clear()
	if q.Total() != 0 || q.Pending() != 0 {
		t.Fatalf("Clear 后应为空，实际 total=%d pending=%d", q.Total(), q.Pending())
	}
}

// 并发入队是真实场景：HTTP 请求 goroutine 放，引擎 goroutine 取。
func TestInboxConcurrentPushDrain(t *testing.T) {
	q := NewInbox()
	const writers = 8
	const perWriter = 50
	const want = writers * perWriter

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				q.Push("msg")
			}
		}()
	}

	// 消费者只通过 channel 回报计数，避免跨 goroutine 共享变量
	counts := make(chan int, 64)
	stop := make(chan struct{})
	var consumer sync.WaitGroup
	consumer.Add(1)
	go func() {
		defer consumer.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if got := q.Drain(); len(got) > 0 {
				counts <- len(got)
			}
		}
	}()

	wg.Wait()
	close(stop)
	// 必须等消费者真正退出后再 close(counts)，否则它可能仍在 counts <- 上阻塞
	consumer.Wait()
	for {
		if len(q.Drain()) == 0 {
			break
		}
		counts <- 0
	}
	close(counts)

	collected := 0
	for n := range counts {
		collected += n
	}
	if collected != want {
		t.Fatalf("并发入队丢失消息: 期望 %d，实际 %d", want, collected)
	}
}

func TestInboxRegistryIsolation(t *testing.T) {
	r := &InboxRegistry{queues: make(map[uint]*Inbox)}
	a := r.Get(1)
	b := r.Get(2)
	a.Push("给会话1")
	if b.Pending() != 0 {
		t.Fatalf("会话间不隔离: 会话2 收到 %d 条", b.Pending())
	}
	if r.Get(1) != a {
		t.Fatal("同一会话应返回同一个队列实例")
	}
	if r.Lookup(3) != nil {
		t.Fatal("Lookup 未注册会话应返回 nil")
	}
	r.Drop(1)
	if r.Lookup(1) != nil {
		t.Fatal("Drop 后应查不到")
	}
}
