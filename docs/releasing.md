# 发布指南

QuizDock 应用与题库相互独立地维护版本和发布，不要求同步升级。

## 发布应用

应用使用标准 `v<主版本>.<次版本>.<修订版本>` 标签，例如 `v0.2.0`。

1. 更新 `VERSION`、`web/package.json` 和 `CHANGELOG.md`。
2. 运行 `make check`。
3. 验证全新数据库启动、登录及至少一个题库导入。
4. 提交并推送应用标签：

   ```bash
   git tag -a v0.2.0 -m "QuizDock v0.2.0"
   git push origin main v0.2.0
   ```

`.github/workflows/release.yml` 会构建 Linux、macOS 和 Windows 可执行文件，生成校验和，并发布 `linux/amd64`、`linux/arm64` Docker 镜像。应用 Release 不再重复打包题库。

## 发布题库

题库标签格式为 `qbank/<banks 下的目录名>/v<题库版本>`，例如：

```text
qbank/software-designer/v0.1.1
```

建议流程：

1. 通过 Issue 或 PR 确认并合并题目勘误。
2. 保持题库 ID 和原题 ID 不变。
3. 只提高该题库 `manifest.json` 的 `version`，不修改应用 `VERSION`。
4. 在本地打包、校验并抽查导入：

   ```bash
   ./dist/quizdock bank pack banks/software-designer release/software-designer-0.1.1.qbank
   ./dist/quizdock bank validate release/software-designer-0.1.1.qbank
   ```

5. 提交勘误和版本变化，然后推送题库标签：

   ```bash
   git tag -a qbank/software-designer/v0.1.1 -m "软件设计师题库 v0.1.1"
   git push origin main qbank/software-designer/v0.1.1
   ```

`.github/workflows/qbank-release.yml` 会验证标签版本与清单一致，打包 `.qbank`，生成 SHA-256 文件，并创建独立的题库 Release。QuizDock 从这类 Release 中发现官方题库，不会把应用标签误认为题库版本。

## 版本原则

- 只改程序：仅提高应用版本。
- 只勘误题目：仅提高对应题库版本。
- 两者都有变化：分别提交并各自发布，可以使用不同版本号和发布时间。
- `minimum_app_version` 只在题库确实需要较新的格式或功能时提高。

题库发布前必须确认其内容许可证允许再分发。许可证为 `Unspecified` 时，工作流技术上仍可运行，但维护者不应公开发布产物。
