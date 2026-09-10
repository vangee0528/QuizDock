# 版本与发布指南

QuizDock 采用**双轨独立版本发布体系 (Dual-Track Release Model)**：应用程序引擎与题库资源拥有独立的版本号、发布节奏与 CI/CD 构建工作流，彼此解耦互不影响。

```text
┌─────────────────────────────────────────────────────────────┐
│                       GitHub 仓库                           │
└──────────────┬───────────────────────────────┬──────────────┘
               │                               │
       推送 v* 标签                    推送 qbank/* 标签
               │                               │
               ▼                               ▼
    ┌──────────────────────┐        ┌──────────────────────┐
    │  应用发布工作流       │        │  题库发布工作流       │
    │  (release.yml)       │        │  (qbank-release.yml) │
    ├──────────────────────┤        ├──────────────────────┤
    │ • 跨平台二进制构建   │        │ • 结构与哈希校验     │
    │ • Docker 镜像推送    │        │ • 打包生成 .qbank    │
    │ • 应用 GitHub Release│        │ • 题库 GitHub Release│
    └──────────────────────┘        └──────────────────────┘
```

## 1. 应用程序发布

应用程序发布包含可执行文件二进制包、源码及 Docker 镜像分发。应用版本遵循标准语义化版本（如 `v0.2.0`）。

### 发布流程 Checklist

1. **版本号同步**：
   - 更新根目录 `VERSION` 文件。
   - 更新 `web/package.json` 中的 `version` 字段。
   - 在 `CHANGELOG.md` 中记录本次版本的变更内容。
2. **本地测试与检查**：
   ```bash
   make check
   ```
3. **冒烟测试**：
   - 验证全新数据库启动正常。
   - 验证认证模式与未认证模式。
   - 验证题库导入与基础做题流程。
4. **提交并推送 Git Tag**：
   ```bash
   git commit -am "chore: release v0.2.0"
   git tag -a v0.2.0 -m "QuizDock v0.2.0"
   git push origin main v0.2.0
   ```

### 自动化产物

推送标签后，`.github/workflows/release.yml` 自动触发执行：
- 编译生成 Linux、macOS (Darwin) 与 Windows 平台的独立二进制归档。
- 生成对应的 SHA-256 校验和文件。
- 构建并发布多架构 Docker 镜像至 GitHub Container Registry (`ghcr.io`)。
- 创建对应的 GitHub Release。

---

## 2. 题库资源发布

官方题库位于 `banks/<bank-dir>/` 目录。题库勘误可随时独立发布，无需更新或重新打包 QuizDock 应用程序。

题库 Git Tag 命名规范为：
```text
qbank/<题库目录名>/v<题库版本>
# 示例：
qbank/software-designer/v0.1.1
```

### 发布流程 Checklist

1. **合并勘误**：审查并合并题目的 Pull Request 或修正。
2. **校验主键约束**：确认题目 ID（如 `q-000555`）保持稳定，未发生篡改或复用。
3. **递增题库版本**：更新对应题库 `manifest.json` 中的 `version` 字段（如 `0.1.1`）。
4. **本地验证构建**：
   ```bash
   # 打包并校验
   quizdock bank pack banks/software-designer dist/software-designer-0.1.1.qbank
   quizdock bank validate dist/software-designer-0.1.1.qbank
   ```
5. **提交并推送题库 Tag**：
   ```bash
   git commit -am "chore(bank): release software-designer v0.1.1"
   git tag -a qbank/software-designer/v0.1.1 -m "软件设计师题库 v0.1.1"
   git push origin main qbank/software-designer/v0.1.1
   ```

### 自动化产物

推送题库标签后，`.github/workflows/qbank-release.yml` 自动触发执行：
- 校验标签版本与 `manifest.json` 中定义的一致性。
- 打包生成标准 `.qbank` 归档文件与对应的 `.qbank.sha256` 校验文件。
- 发布独立的 GitHub Release。QuizDock 客户端可通过 Release API 自动检测并提示用户热更新题库。

---

## 3. 版本与兼容性策略

- **独立发版原则**：
  - 仅改动程序逻辑或界面：仅递增并发布应用版本。
  - 仅题库勘误或题目补充：仅递增并发布对应题库版本。
  - 两者均有改动：按需分别提交对应 Tag，独立发布。
- **向下兼容与 `minimum_app_version`**：
  - 题库通常应保持对旧版本客户端的良好兼容。
  - 仅当题库采用了新版客户端才支持的特定语法特性或扩展字段时，才需在 `manifest.json` 中适度提高 `minimum_app_version`。

题库发布前必须确认其内容许可允许再分发。如果 `license` 仍为 `Unspecified`，工作流虽然可以运行，维护者仍应先完成权利确认。
