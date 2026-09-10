# QuizDock 题库格式规范 (QBank Format v1)

`.qbank` 是 QuizDock 采用的开放题库分发格式。底层基于标准 ZIP 归档容器，封装题库元数据、Markdown 题目、静态资源及 SHA-256 完整性校验和，具有跨平台、纯文本可读与版本可追溯等特性。

## 归档结构规范

标准的 `.qbank` 文件解压后具有如下目录层次结构：

```text
├── manifest.json            # [必填] 题库清单元数据
├── checksums.sha256         # [必填] 全包文件 SHA-256 校验列表
├── questions/               # [必填] 题目 Markdown 文件目录
│   ├── q-000001.md
│   └── ...
└── assets/                  # [可选] 题目引用的静态图片资源
    ├── diagram.webp
    └── ...
```

### 路径与归档约定

1. **相对路径**：所有文件路径统一使用 POSIX 正斜杠 `/`，不得包含绝对路径或 `..` 相对回溯段。
2. **根目录平铺**：上述文件与目录平铺于归档包根目录，不包含额外的顶层包装文件夹。
3. **安全规范**：归档内不包含符号链接（Symlink）、加密条目或上述规范范围之外的文件。
4. **图片格式**：`assets/` 目录下支持 `.png`、`.jpg` (`.jpeg`)、`.gif` 与 `.webp` 静态图片。

## 清单规范 (`manifest.json`)

`manifest.json` 用于描述题库的基础信息与运行要求：

```json
{
  "schema_version": 1,
  "id": "example.software-designer",
  "name": "软件设计师题库",
  "version": "0.1.0",
  "locale": "zh-CN",
  "exam": "示例考试",
  "level": "中级",
  "subject": "软件设计师",
  "question_count": 100,
  "minimum_app_version": "0.1.0",
  "license": "CC-BY-4.0",
  "description": "用于展示 qbank 格式的示例题库。"
}
```

### 字段说明

| 字段 | 类型 | 必填 | 说明 |
| :--- | :--- | :--- | :--- |
| `schema_version` | integer | 是 | 题库规范版本，当前固定为 `1` |
| `id` | string | 是 | 题库全局唯一标识（推荐倒置域名格式，如 `org.example.bank`） |
| `name` | string | 是 | 题库显示名称 |
| `version` | string | 是 | 题库语义化版本（如 `0.1.0` 或 `2026.1.0`） |
| `locale` | string | 否 | 语言与区域标记，默认为 `zh-CN` |
| `exam` | string | 否 | 所属考试名称（如“计算机技术与软件专业技术资格考试”） |
| `level` | string | 否 | 级别分类（如“中级”、“初级”） |
| `subject` | string | 否 | 专业或科目名称（如“软件设计师”） |
| `question_count` | integer | 是 | 题库包含的题目总数 |
| `minimum_app_version` | string | 否 | 所需 QuizDock 应用最低版本要求 |
| `license` | string | 否 | 题库内容开源许可协议或授权类型 |
| `description` | string | 否 | 题库简要说明 |

> 📌 **标识稳定性**：`id` 是该题库在系统内的持久化唯一键。更新发布新版本时，保持相同 `id` 可保证用户的历史答题记录、错题本与个人收藏自动沿用。

## 题目规范 (`questions/*.md`)

每道题目均为独立的 Markdown 文件，文件以题目 ID 命名（如 `q-000001.md`），由 YAML Front-matter 元数据与题目正文构成。

### 单选题示例 (`single-choice`)

````markdown
---
id: q-000001
title: 计算机系统知识-q-000001
chapter: 计算机系统基础
type: single-choice
order: 1
tags: [CPU, 寄存器]
exam: 2024 年上半年
---

# 计算机系统知识-q-000001

## 题目

在 CPU 中，用于暂存正在执行的指令的寄存器是（ ）。

## 选项

- **A.** 程序计数器 (PC)
- **B.** 指令寄存器 (IR)
- **C.** 地址寄存器 (AR)
- **D.** 数据缓冲寄存器 (DR)

## 参考答案

- **B.** 指令寄存器 (IR)

## 解析

指令寄存器（IR）用于暂存当前正在执行的指令，操作码译码器会根据 IR 中的内容分析并产生操作控制信号。
````

### 支持的题型与结构

1. **单选题 (`single-choice`)**：包含 `## 题目`、`## 选项`、`## 参考答案` 与可选的 `## 解析`。
2. **组合题 / 一题多空 (`compound-choice`)**：适用于一篇材料下设多个空（或子问题）的题型，使用 `## 共用题干`、`## 各空选项` 与 `### 第 N 空` 标题结构定义。
3. **材料与解析题 (`content`)**：适用于主观材料或综合题。

### 媒体与排版支持

- **图片引用**：正文中引用题库内图片采用标准 Markdown 相对路径，例如 `![拓扑图](../assets/diagram.webp)`。
- **排版支持**：渲染引擎原生支持 GFM 标准表格、代码块语法高亮。出于安全考虑，渲染层对内联 HTML 脚本进行安全转义。

## 完整性校验 (`checksums.sha256`)

为确保题库分发安全与数据传输完整性，归档包根目录必须包含 `checksums.sha256` 文件。

该文件列出了除其自身以外所有文件（`manifest.json`、`questions/*`、`assets/*`）的标准 SHA-256 校验和：

```text
<64 位小写十六进制字符>  manifest.json
<64 位小写十六进制字符>  questions/q-000001.md
<64 位小写十六进制字符>  assets/diagram.webp
```

## 工具链支持

QuizDock 命令行提供了自动化的题库处理命令，可直接生成规范包与校验和：

```bash
# 从源码目录构建并生成校验和及 .qbank 包
quizdock bank pack ./banks/software-designer ./dist/software-designer.qbank

# 校验题库包结构与哈希完整性
quizdock bank validate ./dist/software-designer.qbank

# 检查题库包元信息与题目统计
quizdock bank inspect ./dist/software-designer.qbank
```
