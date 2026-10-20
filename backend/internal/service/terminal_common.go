package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/minipanel/minipanel/internal/global"
	"github.com/minipanel/minipanel/internal/middleware"
	"github.com/minipanel/minipanel/internal/utils/proxy"
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

// proxyEnvKeys 是需要先清理再注入的代理变量名，避免面板进程残留的旧值叠加。
var proxyEnvKeys = []string{
	"http_proxy", "https_proxy", "all_proxy", "no_proxy",
	"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY",
}

func stripEnvKeys(env []string, keys ...string) []string {
	drop := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		drop[k] = struct{}{}
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name := kv
		if i := strings.IndexByte(kv, '='); i > 0 {
			name = kv[:i]
		}
		if _, bad := drop[name]; bad {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// buildShellEnv 组装终端子进程的环境变量。
//
// 代理是面板的核心增值点：用户在「设置 → 网络」配好代理后，Web 终端里的
// curl / wget / git / apt / pip 应自动走代理，不必手动 export。
// 只在会话创建时读取一次 —— 中途改配置需要重开终端，或按提示执行
// `eval "$(proxy-on)"`，这样已经在跑的长任务不会被环境突变影响。
func buildShellEnv() []string {
	if proxy.IsEnabled() {
		installProxyHelper()
	}

	env := stripEnvKeys(os.Environ(), proxyEnvKeys...)
	env = append(env, "TERM=xterm-256color")

	if cfg := proxy.ActiveConfig(); cfg != nil {
		for k, v := range cfg.EnvVars() {
			env = append(env, k+"="+v)
		}
	}

	// 把面板地址与令牌交给 shell，proxy-on 才能回连面板拉取最新配置。
	// 令牌用本进程的 jwt_secret 现场签发，只存在于该 shell 的环境里，不落盘。
	env = append(env, "PANEL_URL=http://127.0.0.1:"+panelPort())
	if tok := panelToken(); tok != "" {
		env = append(env, "PANEL_TOKEN="+tok)
	}
	return env
}

// panelPort 返回面板监听端口，未配置时回落 8888。
func panelPort() string {
	if global.CONF.Port > 0 {
		return strconv.Itoa(global.CONF.Port)
	}
	return "8888"
}

// panelToken 为终端会话签发一个内部令牌，供 proxy-on 回连面板接口。
//
// 用当前进程的 jwt_secret 现场签发，24 小时过期，权限仅够读代理配置。
func panelToken() string {
	tok, err := middleware.GenerateToken("__terminal__", "admin", 0, []string{"/ssh"})
	if err != nil {
		return ""
	}
	return tok
}

// proxy-on 的可执行文件搜索目录，按优先级排列。
// 家目录通常不在 PATH 中，所以优先写进 /usr/local/bin。
var proxyHelperDirs = []string{
	"/usr/local/bin",
	"/usr/bin",
	"/bin",
	filepath.Join(getHomeDir(), ".local", "bin"),
}

// installProxyHelper 在 PATH 内写一个 proxy-on 脚本。
//
// 脚本本身不缓存配置，而是每次运行时向面板拉最新值，因此面板里改了代理
// 之后，终端里执行 `eval "$(proxy-on)"` 即可就地生效。
func installProxyHelper() {
	port := "8888"
	if global.CONF.Port > 0 {
		port = strconv.Itoa(global.CONF.Port)
	}
	script := "#!/bin/sh\n" +
		"# 由 Mini Panel 生成：输出当前代理配置的 export 语句。\n" +
		"# 用法：eval \"$(proxy-on)\"\n" +
		"if [ -z \"$PANEL_TOKEN\" ]; then\n" +
		"  echo '# 未设置 PANEL_TOKEN，无法读取面板代理配置' >&2\n" +
		"  exit 0\n" +
		"fi\n" +
		"# 回连面板必须绕开代理，否则请求会被发往代理服务器而不是本机。\n" +
		"curl --noproxy '*' -sf \"http://127.0.0.1:" + port + "/api/v1/terminal/proxy-env\" \\\n" +
		"  -H \"Authorization: Bearer ${PANEL_TOKEN}\" 2>/dev/null || \\\n" +
		"  echo '# 面板未响应'\n"

	for _, dir := range proxyHelperDirs {
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, "proxy-on"), []byte(script), 0o755); err == nil {
			return
		}
	}
}

func newExecSession(id string, conn *websocket.Conn, shell string) (*TerminalSession, error) {
	cmd := exec.Command(shell)
	cmd.Dir = getHomeDir()
	cmd.Env = buildShellEnv()

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
			if msgType == websocket.TextMessage && s.looksLikeControl(data) {
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
		if msgType == websocket.TextMessage && s.looksLikeControl(data) {
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

// looksLikeControl 判断一帧是否是前端控制消息（resize 等）。
//
// 比 handleControlMessage 更宽：任何带 type 字段的 JSON 对象整帧都算控制消息。
// 原因是浏览器在 ws.onopen 里发出的 {"type":"auth",token:"..."} 会在服务端
// 读取鉴权帧前残留于连接缓冲，若被当成键盘输入写进 PTY，终端里会出现一串
// 乱码。真正的键盘输入是原始字节流，不会恰好构成这种 JSON 结构。
func (s *TerminalSession) looksLikeControl(data []byte) bool {
	if len(data) == 0 || len(data) > 1024 {
		return false
	}
	var msg struct {
		Type string `json:"type"`
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
	}
	if err := json.Unmarshal(data, &msg); err != nil || msg.Type == "" {
		return false
	}
	if msg.Type == "resize" {
		s.Resize(msg.Cols, msg.Rows)
	}
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
