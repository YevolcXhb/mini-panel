# 🍔Mini Panel🍔

基于1panel管理面板+dockroot开发的轻量级chroot容器面板管理服务，轻量级服务器管理面板，专为 Android Magisk chroot Linux 环境设计。

## 特性

- **Mini Agent AI 智能体**: 基于 ReAct 循环的自然语言运维助手，支持工具调用与执行
  - 多 LLM Provider 支持（OpenAI、Anthropic、DeepSeek、Ollama）
  - Skill 技能插件系统（系统监控、容器管理、网站管理、数据库、防火墙、文件、备份、网络搜索）
  - SSE 流式响应 + 打字机逐字显示效果
  - 破坏性操作安全确认机制
- **系统监控**: CPU、内存、磁盘、网络实时监控
- **文件管理**: Web 文件管理器，支持上传下载
- **Web 终端**: 基于 WebSocket 的交互式终端
- **进程管理**: 查看和结束系统进程
- **容器管理**: 基于 DockRoot 的轻量级容器管理
- **应用商店**: 一键部署常用服务（Nginx、MySQL、Redis 等）
- **计划任务**: Cron 表达式支持的定时任务
- **系统设置**: 灵活的配置管理

## 技术栈

### 后端

- Go 1.23+
- Gin Web 框架
- GORM + SQLite
- DockRoot (容器运行时)
- WebSocket (终端)
- ReAct Agent 引擎（SSE 流式响应、工具调用、上下文压缩）
- 多 LLM Provider 抽象层（OpenAI 兼容协议）

### 前端

- Vue 3 + TypeScript
- Element Plus UI
- Vite 构建工具
- Axios HTTP 客户端
- xterm.js (终端组件)
- SSE ReadableStream 流式消费
- 打字机逐字显示效果

## 快速开始

### 一键部署（推荐）

> 国内用户若直连 GitHub 失败，请依次尝试下方镜像命令（镜像可用性随时间变化，哪个能用就用哪个）。

```bash
# 主推：gh-proxy.com 镜像
curl -fsSL "https://gh-proxy.com/https://raw.githubusercontent.com/YevolcXhb/mini-panel/main/install.sh" -o /tmp/install.sh && bash /tmp/install.sh
```

```bash
# 备选1：ghproxy.cn 镜像
curl -fsSL "https://ghproxy.cn/https://raw.githubusercontent.com/YevolcXhb/mini-panel/main/install.sh" -o /tmp/install.sh && bash /tmp/install.sh
```

```bash
# 备选2：ghps.cc 镜像
curl -fsSL "https://ghps.cc/https://raw.githubusercontent.com/YevolcXhb/mini-panel/main/install.sh" -o /tmp/install.sh && bash /tmp/install.sh
```

```bash
# 备选3：手动下载（镜像全挂时的兜底）
# 浏览器打开 https://github.com/YevolcXhb/mini-panel/releases 下载对应架构的
# minipanel-<版本>-linux-<架构>.tar.gz，解压后执行包内自带的 install.sh 即可
# （release 包内已包含 install.sh，见 scripts/release.sh:36 和 .github/workflows/release.yml:126）
```

<details>
<summary>网络畅通环境（GitHub 直连）</summary>

```bash
curl -fsSL "https://raw.githubusercontent.com/YevolcXhb/mini-panel/main/install.sh" | bash
```

</details>

### 从源码构建

#### 环境要求

- Go 1.23+
- Node.js 18+
- DockRoot (可选，用于容器管理)

#### 后端启动

```bash
cd backend
cp config.yaml.example config.yaml
go mod tidy
go run cmd/server/main.go
```

#### 前端开发

```bash
cd frontend
npm install
npm run dev
```

#### 构建

```bash
# 后端
make build

# 前端
npm run build
```

#### Android 构建

```bash
cd backend
make build_android
```

## 默认登录

- 用户名: `admin`
- 密码: `admin123`

## 项目结构

```
mini-panel/
├── backend/              # Go 后端
│   ├── cmd/
│   │   ├── server/       # mini-panel 入口
│   │   └── dockroot/     # DockRoot 源码（独立模块）
│   ├── internal/
│   │   ├── agent/        # Mini Agent AI 智能体
│   │   │   ├── provider/ # LLM Provider 抽象（OpenAI/Anthropic/Ollama）
│   │   │   ├── tools/    # 工具实现（容器、网站、数据库、防火墙等）
│   │   │   ├── skills/   # Skill 技能插件系统
│   │   │   ├── repository/ # Agent 会话与配置仓库
│   │   │   ├── engine.go # ReAct 引擎核心
│   │   │   └── factory.go # 引擎工厂
│   │   ├── api/          # HTTP 处理器
│   │   ├── service/      # 业务逻辑
│   │   ├── model/        # 数据模型
│   │   ├── repository/   # 数据访问
│   │   ├── router/       # 路由
│   │   ├── middleware/   # 中间件（含安全入口）
│   │   ├── dto/          # 数据传输对象
│   │   ├── global/       # 全局变量
│   │   ├── config/       # 配置管理
│   │   └── utils/
│   │       ├── dockroot/ # DockRoot 客户端
│   │       ├── cmd/      # 命令执行
│   │       └── psutil/   # 系统监控
│   ├── config.yaml.example
│   ├── go.mod
│   └── Makefile
├── frontend/             # Vue3 + Element Plus 前端
│   ├── src/
│   │   ├── views/        # 页面组件
│   │   │   ├── MiniAgent.vue   # AI 智能体对话界面
│   │   │   ├── Terminal.vue    # Web 终端
│   │   │   └── SSH.vue         # SSH 管理
│   │   ├── api/          # API 封装
│   │   ├── router/       # 路由
│   │   ├── store/        # Pinia 状态管理
│   │   └── styles/       # 暗色主题
│   ├── package.json
│   └── vite.config.ts
├── .github/workflows/    # CI/CD 自动构建
├── install.sh            # 一键安装脚本
└── README.md
```

## Agent 智能体说明

Mini Agent 是集成在 Mini Panel 中的 AI 运维助手，基于 ReAct 循环实现自然语言控制服务器：

- **多模型支持**: OpenAI、Anthropic (Claude)、DeepSeek、Ollama（本地模型）
- **Skill 技能系统**: 按需启用系统监控、容器、网站、数据库、防火墙、文件、备份、网络搜索等技能
- **安全确认**: 对删除、停止、重启、kill 等破坏性操作会自动要求用户确认
- **上下文管理**: 自动会话历史保存与压缩，支持长对话
- **流式交互**: SSE 实时流式响应 + 打字机逐字显示效果

使用方式：进入面板左侧菜单「Mini Agent」，配置 LLM Provider 后即可通过自然语言进行运维操作。

## 容器管理说明

Mini Panel 使用 [DockRoot](https://github.com/kspeeder/dockroot) 作为容器运行时，支持：

- 拉取 Docker 镜像
- 运行/停止/删除容器
- 查看容器日志
- 绑定挂载卷

限制：

- 仅支持 host 网络模式
- 不支持 Docker Compose
- 不支持镜像构建

## License

AGPL-3.0 License

## 致谢

在这里感谢 1Panel 官方以及 DockRoot 开发者开源的源代码。

- [1Panel](https://github.com/1Panel-dev/1Panel)
- [DockRoot](https://github.com/kspeeder/dockroot)
