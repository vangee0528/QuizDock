# 软件设计师题库 (`cn.ruankao.software-designer`)

本目录收录全国计算机技术与软件专业技术资格（水平）考试——**中级软件设计师**科目题库源文件。

题库按 QuizDock 标准格式组织，当前包含 2,467 道历年真题及模拟练习题，覆盖计算机体系结构、操作系统、软件工程、面向对象与设计模式、数据库、计算机网络与信息安全、数据结构与算法、标准化与知识产权等核心知识领域。

## 目录结构

- `manifest.json`：题库清单元数据（题库标识、版本、科目分类、题目总数等）。
- `questions/`：题目 Markdown 源文件，采用全局稳定标识（如 `q-000001.md`）。
- `assets/`：题目所引用的拓扑图、电路图、UML 图等静态资源图片。

## 构建与校验

可通过 QuizDock CLI 工具将源码目录打包为分发专用的 `.qbank` 归档文件：

```bash
# 检查题目结构、答案引用、资源路径和敏感来源信息
node tools/audit-qbank.mjs banks/software-designer

# 从当前目录打包为 .qbank 文件
quizdock bank pack . ../../dist/software-designer-0.2.0.qbank

# 校验生成的题库包完整性与 SHA-256 校验和
quizdock bank validate ../../dist/software-designer-0.2.0.qbank
```

## 勘误与贡献

欢迎协助完善题库质量！若在备考刷题中发现题干笔误、排版遗漏、选项错位或参考答案存疑，欢迎提交 Issue 或 Pull Request：

1. **定位题目**：在 Web 界面中可直接查看题目 ID（例如 `q-000555`），在 `questions/` 目录下检索对应 Markdown 文件进行修正。
2. **保持 ID 稳定**：题目 ID 是用户做题记录、收藏及错题本持久化的主键，勘误时**请勿修改已有的题目 ID**。
3. **资源图片**：若涉及配图增补或修正，请统一放置于 `assets/` 目录并使用 Markdown 相对路径引用。

## 版本与发布

本题库遵循独立的语义化版本体系。题库勘误合并后，更新 `manifest.json` 中的 `version` 字段并推送对应 Git 标签（例如 `qbank/software-designer/v0.1.1`），GitHub Actions 即可自动完成校验、打包与发布。

## 许可说明

题库内容许可目前标记为 `Unspecified`。在确认题目及图片的再分发权利并选择正式许可前，不应对外发布题库包。QuizDock 应用代码独立使用 MIT 许可证。
