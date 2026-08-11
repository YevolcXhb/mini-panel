package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

type TerminalSession struct {
	conn   *websocket.Conn
	pty    *os.File
	stdin  io.WriteCloser
	stdout io.ReadCloser
	cmd    *exec.Cmd
	mu     sync.Mutex
	closed bool
	usePTY bool
	role   string
}

var (
	sessions   = make(map[string]*TerminalSession)
	sessionsMu sync.RWMutex
)

// allowedCommands 普通用户允许执行的只读命令白名单。
// 仅包含查看类/文本处理类命令，任何会修改文件、进程、网络或系统的命令均被拒绝。
var allowedCommands = map[string]bool{
	// 目录与文件查看
	"ls": true, "ll": true, "la": true, "l": true, "cd": true, "pwd": true,
	"cat": true, "head": true, "tail": true, "tac": true, "nl": true,
	"less": true, "more": true, "tree": true, "stat": true, "file": true,
	// 文本处理（只读）
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "ag": true,
	"wc": true, "sort": true, "uniq": true, "diff": true, "cmp": true,
	"comm": true, "cut": true, "tr": true, "paste": true, "column": true,
	"fold": true, "fmt": true, "expand": true, "unexpand": true, "rev": true,
	// 查找与定位
	"find": true, "locate": true, "which": true, "whereis": true, "type": true,
	"readlink": true, "realpath": true, "basename": true, "dirname": true,
	// 系统信息（只读）
	"uname": true, "hostname": true, "date": true, "cal": true, "uptime": true,
	"who": true, "whoami": true, "id": true, "groups": true, "nproc": true,
	"lscpu": true, "lsblk": true, "lsusb": true, "lspci": true, "lsmod": true,
	"lsattr": true, "df": true, "du": true, "free": true, "vmstat": true,
	"top": true, "htop": true, "ps": true, "w": true, "lsof": true,
	"netstat": true, "ss": true, "ifconfig": true, "dmesg": true, "last": true,
	"getfacl": true, "getfattr": true, "locale": true,
	// 哈希与编码
	"md5sum": true, "sha1sum": true, "sha256sum": true, "sha512sum": true,
	"cksum": true, "sum": true, "base32": true, "base64": true,
	"xxd": true, "od": true, "hexdump": true, "strings": true,
	// 其他只读工具
	"echo": true, "printf": true, "env": true, "printenv": true, "export": true,
	"history": true, "clear": true, "man": true, "help": true, "info": true,
	"bc": true, "seq": true, "factor": true,
	"true": true, "false": true, "test": true, "hash": true,
	"tput": true, "exit": true, "logout": true,
}

// isCommandAllowed 检查普通用户输入的命令行是否全部允许。
// 返回 (allowed, rejectedDesc)，rejectedDesc 为拒绝原因或被拒命令名。
func isCommandAllowed(line string) (bool, string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return true, ""
	}
	// 禁止重定向（防止写入/覆盖文件）
	if strings.ContainsAny(line, "<>") {
		return false, "禁止使用重定向 (< > >> <<)"
	}
	// 禁止命令替换（防止注入绕过）
	if strings.Contains(line, "`") || strings.Contains(line, "$(") {
		return false, "禁止使用命令替换 (` ` 或 $())"
	}
	// 按管道、逻辑运算符、分号分割，逐段检查每条命令
	for _, part := range splitPipeline(line) {
		cmd := extractCommand(part)
		if cmd == "" {
			continue
		}
		if !allowedCommands[cmd] {
			return false, cmd
		}
		// find 本身只读，但 -exec/-delete/-ok/-fprint 等参数可执行命令或写文件
		if cmd == "find" {
			for _, arg := range strings.Fields(part) {
				low := strings.ToLower(arg)
				if strings.HasPrefix(low, "-exec") || strings.HasPrefix(low, "-ok") ||
					low == "-delete" || strings.HasPrefix(low, "-fprint") ||
					low == "-fls" || low == "-printf" {
					return false, "find 参数被禁止: " + arg
				}
			}
		}
	}
	return true, ""
}

// splitPipeline 按管道符、&&、||、; 分割命令行
func splitPipeline(line string) []string {
	line = strings.ReplaceAll(line, "&&", "\x00")
	line = strings.ReplaceAll(line, "||", "\x00")
	line = strings.ReplaceAll(line, "|", "\x00")
	line = strings.ReplaceAll(line, ";", "\x00")
	return strings.Split(line, "\x00")
}

// extractCommand 从一段命令中提取命令名（跳过前导环境变量赋值，去除路径前缀）
func extractCommand(part string) string {
	part = strings.TrimSpace(part)
	fields := strings.Fields(part)
	for _, f := range fields {
		// 跳过环境变量赋值 (NAME=value)
		if idx := strings.Index(f, "="); idx > 0 {
			if isValidIdent(f[:idx]) {
				continue
			}
		}
		cmd := f
		// 去除路径前缀 (/usr/bin/ls -> ls)
		if idx := strings.LastIndex(cmd, "/"); idx >= 0 {
			cmd = cmd[idx+1:]
		}
		return cmd
	}
	return ""
}

func isValidIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

func getHomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return u.HomeDir
	}
	return "/"
}

func newExecSession(id string, conn *websocket.Conn, shell string) (*TerminalSession, error) {
	cmd := exec.Command(shell)
	cmd.Dir = getHomeDir()
	cmd.Env = append(os.Environ(), "TERM=xterm")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start shell: %w", err)
	}

	sess := &TerminalSession{
		conn:   conn,
		stdin:  stdin,
		stdout: stdout,
		cmd:    cmd,
		usePTY: false,
	}

	sessionsMu.Lock()
	sessions[id] = sess
	sessionsMu.Unlock()

	go sess.readLoop()
	go sess.writeLoop()

	return sess, nil
}

func (s *TerminalSession) readLoop() {
	buf := make([]byte, 1024)
	var reader io.Reader
	if s.usePTY {
		reader = s.pty
	} else {
		reader = s.stdout
	}

	for {
		n, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				s.conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n[disconnect: %v]\r\n", err)))
			}
			break
		}
		s.conn.WriteMessage(websocket.BinaryMessage, buf[:n])
	}
	s.Close()
}

// writeLoop 读取前端输入并写入 shell。
// 管理员无限制；普通用户启用命令白名单过滤：逐行缓冲，回车时检查整行命令，
// 危险/非只读命令不予执行并回显红色拦截提示。
// 对方向键、Tab 补全等不可追踪输入标记 dirty，整行不再信任，要求用户手动输入。
func (s *TerminalSession) writeLoop() {
	var writer io.Writer
	if s.usePTY {
		writer = s.pty
	} else {
		writer = s.stdin
	}

	// 管理员：直接透传
	if s.role == "admin" {
		for {
			msgType, data, err := s.conn.ReadMessage()
			if err != nil {
				break
			}
			if msgType == websocket.TextMessage && s.handleControlMessage(data) {
				continue
			}
			writer.Write(data)
		}
		s.Close()
		return
	}

	// 普通用户：行缓冲命令过滤
	var lineBuf []byte
	dirty := false      // 当前行包含不可追踪输入（方向键/Tab/历史），不再信任 lineBuf
	escapeMode := false // 处理 ESC 转义序列

	resetLine := func() {
		lineBuf = lineBuf[:0]
		dirty = false
		escapeMode = false
	}
	reject := func(msg string) {
		// Ctrl+U 清除当前行输入，回车换行，输出红色警告
		writer.Write([]byte("\x15\r\n"))
		s.conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\x1b[31m[已拦截] %s\x1b[0m\r\n", msg)))
		resetLine()
	}

	for {
		msgType, data, err := s.conn.ReadMessage()
		if err != nil {
			break
		}
		if msgType == websocket.TextMessage && s.handleControlMessage(data) {
			continue
		}
		for i := 0; i < len(data); i++ {
			b := data[i]

			if escapeMode {
				writer.Write([]byte{b})
				// CSI 序列以字母结尾（ESC [ params letter）
				if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
					escapeMode = false
				}
				continue
			}

			switch b {
			case 0x1b: // ESC：进入转义序列，标记 dirty（方向键/功能键不可追踪）
				writer.Write([]byte{b})
				escapeMode = true
				dirty = true
			case '\r', '\n': // 回车：检查命令
				if dirty {
					reject("请手动输入命令，禁止使用方向键/Tab 补全调出历史命令")
					continue
				}
				allowed, cmd := isCommandAllowed(string(lineBuf))
				if !allowed {
					reject(fmt.Sprintf("危险或非只读命令被禁止: %s", cmd))
					continue
				}
				// 允许：写入换行符执行命令
				writer.Write([]byte{b})
				resetLine()
			case 0x03: // Ctrl+C：中断当前命令
				writer.Write([]byte{b})
				resetLine()
			case 0x15: // Ctrl+U：清行
				writer.Write([]byte{b})
				resetLine()
			case 0x7f, '\b': // 退格
				writer.Write([]byte{b})
				if len(lineBuf) > 0 {
					lineBuf = lineBuf[:len(lineBuf)-1]
				}
			case 0x09: // Tab：补全结果不可追踪
				writer.Write([]byte{b})
				dirty = true
			default:
				writer.Write([]byte{b})
				if b >= 0x20 && !dirty {
					lineBuf = append(lineBuf, b)
				}
			}
		}
	}
	s.Close()
}

// handleControlMessage 处理前端发来的控制消息（目前只有终端尺寸调整 resize）
func (s *TerminalSession) handleControlMessage(data []byte) bool {
	var msg struct {
		Type string `json:"type"`
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
	}
	if err := json.Unmarshal(data, &msg); err != nil || msg.Type != "resize" {
		return false
	}
	s.Resize(msg.Cols, msg.Rows)
	return true
}

func (s *TerminalSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
	if s.pty != nil {
		s.pty.Close()
	}
	if s.stdin != nil {
		s.stdin.Close()
	}
	if s.stdout != nil {
		s.stdout.Close()
	}
	s.conn.Close()
}

func (s *TerminalSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func GetSession(id string) (*TerminalSession, bool) {
	sessionsMu.RLock()
	defer sessionsMu.RUnlock()
	sess, ok := sessions[id]
	return sess, ok
}

func RemoveSession(id string) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if sess, ok := sessions[id]; ok {
		sess.Close()
		delete(sessions, id)
	}
}
