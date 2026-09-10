# 系统架构

QuizDock 是一款本地优先的 Web 应用，可以打包为一个 Go 可执行文件。程序内只嵌入编译后的前端和数据库迁移，题目通过用户选择的 `.qbank` 导入。官方题库源与应用代码位于同一仓库，但不会嵌入可执行文件或 Docker 镜像。

```text
浏览器（React + TypeScript）
          │ JSON / multipart
          ▼
Go HTTP 服务
  ├── 可选登录与会话
  ├── qbank 校验、下载和导入
  ├── 练习与错题复习逻辑
  ├── 应用和题库更新发现
  ├── 内嵌前端
  └── SQLite 数据库
```

## 模块边界

- `cmd/quizdock`：命令行参数与进程生命周期。
- `internal/api`：HTTP 传输、登录、更新入口和静态前端服务。
- `internal/qbank`：公开的题库压缩包及 Markdown 格式。
- `internal/releases`：解析 GitHub Release，区分应用与题库版本。
- `internal/database`：数据库迁移、题库目录和学习状态。
- `web`：浏览器交互与可恢复的练习会话。

## 稳定标识

题目由 `bank_id + question_id` 唯一标识。发布者修正或更新题目时必须保持两者不变。题库导入在一个 SQLite 事务中完成，不删除答题记录、进度、收藏和掌握状态。

## 页面恢复

当前模式和题目写入 `/practice/:mode/:uid`，筛选条件写入查询参数，长期进度保存到 SQLite，当前队列和滚动位置保存到 `sessionStorage`。因此刷新页面可以恢复到原题，随机模式的题目顺序也能保持。

## 信任与网络边界

服务默认只监听 `127.0.0.1`。设置登录密码后，除健康检查与登录接口外的 API 都要求有效的 HttpOnly 会话 Cookie。公网部署应同时配置 HTTPS 反向代理；登录功能不替代 TLS、防火墙或系统安全更新。
