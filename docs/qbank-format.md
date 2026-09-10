# QuizDock 题库格式 v1

`.qbank` 是格式稳定、与应用实现无关的 ZIP 压缩包，包含清单、Markdown 题目、可选图片资源和校验和列表。

## 目录结构

```text
manifest.json
checksums.sha256
questions/q-000001.md
assets/diagram.webp
```

路径统一使用 `/`，必须是相对路径且不能包含 `..`。导入器拒绝符号链接、加密条目和三个规定区域之外的文件。v1 支持 PNG、JPEG、GIF 和 WebP 图片。

## 清单

```json
{
  "schema_version": 1,
  "id": "example.software-designer",
  "name": "软件设计师题库",
  "version": "2026.1.0",
  "locale": "zh-CN",
  "exam": "计算机技术与软件专业技术资格",
  "level": "中级",
  "subject": "软件设计师",
  "question_count": 100,
  "minimum_app_version": "0.1.0",
  "license": "CC-BY-4.0"
}
```

`id` 和每一道题的 ID 都是永久标识。发布者不得将它们复用于无关内容。使用相同 `id` 更新题库时，用户的答题记录和进度会保留。

题库 `version` 独立于 QuizDock 应用版本。推荐使用三段式语义版本，并在任何题目或资源发生变化时提高版本。

## 题目 Markdown

````markdown
---
id: q-000001
title: 计算机基础-q-000001
chapter: 计算机基础
type: single-choice
order: 1
tags: [CPU, 寄存器]
exam: 2025 年上半年
---

# 计算机基础-q-000001

## 题目

在 CPU 中，跟踪下一条要执行的指令地址的寄存器是（ ）。

## 选项

- **A.** PC
- **B.** MAR
- **C.** MDR
- **D.** IR

## 参考答案

- **A.** PC
````

v1 支持 `single-choice`、`compound-choice` 和 `content`。组合题使用 `## 共用题干`、`## 各空选项` 和 `### 第 N 空` 标题。

图片使用相对 Markdown 路径，例如 `![网络图](../assets/network.webp)`。网页渲染器不会执行原始 HTML。

## 校验和

`checksums.sha256` 必须为清单、每道题和每个资源各包含一行 SHA-256；校验文件自身不写入列表。

```text
<64 位小写十六进制字符>  manifest.json
<64 位小写十六进制字符>  questions/q-000001.md
```

使用 `quizdock bank pack` 可以确定性地生成校验文件和压缩包。
