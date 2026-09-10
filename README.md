# QuizDock

<p align="center">
  <img src="web/public/favicon.svg" alt="QuizDock Logo" width="80" height="80" />
</p>

<p align="center">
  <strong>现代化、本地优先（Local-First）的轻量刷题工具与题库平台</strong>
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  <a href="https://github.com/vangee0528/QuizDock/releases"><img src="https://img.shields.io/github/v/release/vangee0528/QuizDock" alt="Latest Release"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.27+-00ADD8.svg" alt="Go Version"></a>
  <a href="https://react.dev/"><img src="https://img.shields.io/badge/React-18-61DAFB.svg" alt="React Version"></a>
</p>

---

QuizDock 是一款本地优先、开箱即用的轻量刷题与备考辅助工具。

系统采用**核心引擎与题库资源解耦**的设计理念：应用程序不与特定考试或科目绑定，而是通过开放的 `.qbank` 格式统一分发题库。你可以根据学习需求自由导入、组合、启用或更新多个题库。

## ✨ 核心特性

- 🧩 **题库解耦与热插拔**：基于标准 `.qbank` 格式，支持通过 Web 界面或 CLI 导入与管理题库，题库可按需单独启用或停用。
- 📚 **多元练习模式**：内置顺序练习、章节分类、随机抽题、历年真题及错题专练等多种刷题场景。
- ⚡ **无感状态恢复**：答题进度、错题本与个人收藏实时持久化，支持路由定位与页面刷新无缝还原。
- 📝 **高质量富文本排版**：原生支持 GFM Markdown 表格、代码高亮以及题库本地图文混排。
- 🤖 **AI 辅助解题**：答题后支持一键提取题目上下文，快捷唤起 AI 助手进行题目深度解析。
- 🔐 **轻量访问控制**：内置可选的账号密码保护，无需复杂配置即可安全部署于 VPS 或公网服务器。
- 🔄 **双轨独立更新**：应用主体与题库内容版本解耦，题目勘误支持单独发布与在线热更新。
- 📦 **单二进制极简交付**：基于 Go + React + SQLite 构建，内嵌 Web 界面与数据库迁移，零外部依赖开箱即用。

## 🚀 快速上手

### 预编译程序运行（推荐）

从 [GitHub Releases](https://github.com/vangee0528/QuizDock/releases) 下载对应系统的压缩包，解压后直接启动：

```bash
./quizdock serve
```

服务启动后将监听 `http://127.0.0.1:8765` 并自动打开浏览器。数据默认保存在系统标准的用户应用数据目录下，可通过 `--data-dir` 自定义存储路径。

### Docker 部署

使用 Docker Compose 快速启动：

```bash
docker compose up -d
```

或直接运行官方镜像：

```bash
docker run -d \
  --name quizdock \
  -p 8765:8765 \
  -v quizdock-data:/data \
  --restart unless-stopped \
  ghcr.io/vangee0528/quizdock:0.2.1
```

### 源码编译与开发

环境要求：Go 1.27+、Node.js 22+、npm 10+。

```bash
# 初始化并安装依赖
make setup

# 启动前后端联合开发模式（支持热重载）
make dev

# 编译单文件生产版本
make build
./dist/quizdock serve
```

## 🔒 身份认证与服务部署

QuizDock 支持通过环境变量或命令行参数配置访问凭据。未配置密码时保持本地免登录模式；配置凭据后将自动开启 Web 登录验证。

**环境变量配置：**

```bash
export QUIZDOCK_AUTH_USER=admin
export QUIZDOCK_AUTH_PASSWORD="YourSecurePassword"
./quizdock serve --host 0.0.0.0 --no-browser
```

**凭据文件配置（适用于生产环境）：**

```bash
./quizdock serve --host 0.0.0.0 --no-browser \
  --auth-user admin \
  --auth-password-file /etc/quizdock/password
```

在 Docker Compose 部署中，可直接在同目录下的 `.env` 文件中设置 `QUIZDOCK_AUTH_USER` 与 `QUIZDOCK_AUTH_PASSWORD`。

> 💡 **部署建议**：公网访问场景下，推荐使用 Nginx、Caddy 等反向代理工具配置 HTTPS/TLS 证书。

## 📦 题库管理命令行 (CLI)

QuizDock 提供了完整的题库操作工具链：

```bash
# 校验题库包格式与 SHA-256 完整性
quizdock bank validate software-designer.qbank

# 查看题库包元信息与题目统计
quizdock bank inspect software-designer.qbank

# 导入题库至本地数据库
quizdock bank import software-designer.qbank

# 从本地源码目录打包生成 .qbank 文件
quizdock bank pack ./banks/software-designer ./software-designer.qbank
```

题库规范与编写标准详见 [题库格式文档](docs/qbank-format.md)。

## 📁 目录结构

```text
├── cmd/quizdock/            # CLI 命令行与服务入口
├── internal/
│   ├── api/                 # HTTP API、登录认证、更新检查与静态资源服务
│   ├── database/            # SQLite 存储、数据模型与迁移逻辑
│   ├── qbank/               # .qbank 解析、校验与打包引擎
│   └── releases/            # 官方更新检测服务
├── web/                     # React + TypeScript 前端单页应用
├── banks/                   # 题库源码仓库
│   └── software-designer/   # 官方《软件设计师》题库
└── docs/                    # 系统设计与规范文档
```

## 🛡️ 数据持久化机制

QuizDock 采用 SQLite 存储题库与用户学习数据。在数据模型设计上，题库内容与用户做题状态（答题历史、做题进度、错题本、收藏状态）逻辑隔离——题库版本升级仅更新题目与目录数据，卸载题库亦会完整保留用户的学习历史记录。

## 📄 开源许可证

- **应用程序**：遵循 [MIT License](LICENSE) 开源。
- **题库内容**：拥有独立的内容授权，请参考各题库根目录的 `manifest.json` 及相应许可说明。
