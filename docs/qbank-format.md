# QuizDock question-bank format v1

A `.qbank` file is a ZIP archive with a stable, application-independent format. It contains a manifest, Markdown questions, optional image assets and a checksum list.

## Layout

```text
manifest.json
checksums.sha256
questions/q-000001.md
assets/diagram.webp
```

Paths use `/`, are relative and may not contain `..`. Symlinks, encrypted entries and files outside the three documented areas are rejected. The v1 importer accepts PNG, JPEG, GIF and WebP assets.

## Manifest

```json
{
  "schema_version": 1,
  "id": "example.software-designer",
  "name": "软件设计师题库",
  "version": "2026.1",
  "locale": "zh-CN",
  "exam": "计算机技术与软件专业技术资格",
  "level": "中级",
  "subject": "软件设计师",
  "question_count": 100,
  "minimum_app_version": "0.1.0",
  "license": "CC-BY-4.0"
}
```

`id` and every question ID are permanent identities. Publishers must not reuse them for unrelated content. Updating a bank with the same `id` preserves the user's attempts and progress.

## Question Markdown

```markdown
---
id: q-000001
title: CPU 指令地址寄存器
chapter: 计算机基础
type: single-choice
order: 1
tags: [CPU, 寄存器]
exam: 2025 年上半年
---

# CPU 指令地址寄存器

## 题目

在 CPU 中，跟踪下一条要执行的指令地址的寄存器是（ ）。

## 选项

- **A.** PC
- **B.** MAR
- **C.** MDR
- **D.** IR

## 参考答案

- **A.** PC
```

Supported types in v1 are `single-choice`, `compound-choice` and `content`. Compound questions use `## 共用题干`, `## 各空选项` and `### 第 N 空` headings.

Images use relative Markdown references such as `![网络图](../assets/network.webp)`. Raw HTML is treated as text by the web renderer.

## Checksums

`checksums.sha256` contains one SHA-256 line for every manifest, question and asset file. The checksum file itself is not listed.

```text
<64 lowercase hexadecimal characters>  manifest.json
<64 lowercase hexadecimal characters>  questions/q-000001.md
```

Use `quizdock bank pack` to generate this file and the archive deterministically.
