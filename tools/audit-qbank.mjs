#!/usr/bin/env node

import { existsSync, readFileSync, readdirSync } from "node:fs";
import { basename, dirname, join, relative, resolve } from "node:path";

const bankDirectory = resolve(process.argv[2] ?? "banks/software-designer");
const questionDirectory = join(bankDirectory, "questions");
const manifestFile = join(bankDirectory, "manifest.json");
const failures = [];
const warnings = [];
const duplicateQuestions = new Map();

function report(collection, file, message) {
  collection.push(`${relative(bankDirectory, file)}: ${message}`);
}

function parseFrontMatter(file, text) {
  if (!text.startsWith("---\n")) {
    report(failures, file, "缺少 YAML front matter");
    return { metadata: {}, body: text };
  }
  const end = text.indexOf("\n---\n", 4);
  if (end < 0) {
    report(failures, file, "YAML front matter 未闭合");
    return { metadata: {}, body: text };
  }
  const metadata = {};
  for (const line of text.slice(4, end).split("\n")) {
    const match = line.match(/^([a-z_]+):\s*(?:"([^"]*)"|(.*))$/);
    if (match) metadata[match[1]] = (match[2] ?? match[3]).trim();
  }
  return { metadata, body: text.slice(end + 5) };
}

function splitSections(body) {
  const result = new Map();
  const matches = [...body.matchAll(/^##\s+(.+?)\s*$/gm)];
  matches.forEach((match, index) => {
    const start = match.index + match[0].length;
    const end = matches[index + 1]?.index ?? body.length;
    result.set(match[1].trim(), body.slice(start, end).trim());
  });
  return result;
}

function optionEntries(text) {
  const matches = [...text.matchAll(/^\s*-\s+\*\*([A-Z])[.、．:]\*\*\s*(.*)$/gm)];
  return matches.map((match, index) => ({
    label: match[1],
    body: text
      .slice(match.index + match[0].length - match[2].length, matches[index + 1]?.index ?? text.length)
      .trim(),
  }));
}

function normalized(value) {
  return value
    .replace(/\*\*/g, "")
    .replace(/`/g, "")
    .replace(/[，。；：、（）\s]/g, "")
    .toLowerCase();
}

function validateOptions(file, optionText, answerText, label = "本题") {
  const options = optionEntries(optionText);
  const answers = optionEntries(answerText);
  const labels = options.map((option) => option.label);
  if (options.length < 2) report(failures, file, `${label}少于两个选项`);
  if (new Set(labels).size !== labels.length) report(failures, file, `${label}存在重复选项标签`);
  if (answers.length !== 1) report(failures, file, `${label}应有且仅有一个参考答案，实际为 ${answers.length}`);
  for (const answer of answers) {
    const option = options.find((item) => item.label === answer.label);
    if (!option) {
      report(failures, file, `${label}答案 ${answer.label} 不在选项中`);
    } else if (normalized(option.body) !== normalized(answer.body)) {
      report(failures, file, `${label}答案 ${answer.label} 的文本与对应选项不一致`);
    }
  }
  const bodies = new Map();
  for (const option of options) {
    const key = normalized(option.body);
    if (key && bodies.has(key)) report(warnings, file, `${label}选项 ${bodies.get(key)} 与 ${option.label} 内容重复`);
    else if (key) bodies.set(key, option.label);
  }
}

function validateCompound(file, sections) {
  const optionText = sections.get("各空选项") ?? "";
  const answerText = sections.get("参考答案") ?? "";
  const parts = [...optionText.matchAll(/^###\s+(.+?)\s*$/gm)];
  if (parts.length < 2) report(failures, file, "组合选择题少于两个分题");
  parts.forEach((part, index) => {
    const start = part.index + part[0].length;
    const end = parts[index + 1]?.index ?? optionText.length;
    const number = index + 1;
    const answerMatch = answerText.match(
      new RegExp(`^\\s*-\\s+第\\s*${number}\\s*空[：:]\\s*\\*\\*([A-Z])[.、．:]\\*\\*\\s*(.*)$`, "m"),
    );
    const partAnswer = answerMatch ? `- **${answerMatch[1]}.** ${answerMatch[2]}` : "";
    validateOptions(file, optionText.slice(start, end), partAnswer, `第 ${number} 空`);
  });
}

if (!existsSync(questionDirectory)) {
  console.error(`题目目录不存在：${questionDirectory}`);
  process.exit(2);
}

const files = readdirSync(questionDirectory)
  .filter((name) => /^q-\d{6}\.md$/.test(name))
  .sort()
  .map((name) => join(questionDirectory, name));

try {
  const manifest = JSON.parse(readFileSync(manifestFile, "utf8"));
  if (manifest.question_count !== files.length) {
    report(failures, manifestFile, `question_count 为 ${manifest.question_count}，实际题目数为 ${files.length}`);
  }
} catch (error) {
  report(failures, manifestFile, `无法读取 manifest.json：${error.message}`);
}

for (const file of files) {
  const text = readFileSync(file, "utf8").replaceAll("\r\n", "\n");
  const { metadata, body } = parseFrontMatter(file, text);
  const expectedID = basename(file, ".md");
  if (metadata.id !== expectedID) report(failures, file, `id 应为 ${expectedID}，实际为 ${metadata.id ?? "空"}`);
  if (Number(metadata.order) !== Number(expectedID.slice(2))) report(failures, file, "order 与文件名题号不一致");
  if (!metadata.title?.endsWith(expectedID)) report(failures, file, "title 应以稳定题目 ID 结尾");

  const fences = (body.match(/^```/gm) ?? []).length;
  if (fences % 2 !== 0) report(failures, file, "代码围栏未成对闭合");
  if (/KnowSeeker|soupcola|51cto/i.test(text)) report(failures, file, "包含应移除的来源标识");
  if (/^\s*-\s*["']?complete["']?\s*$/im.test(text)) report(failures, file, "包含 complete tag");
  if (/抽奖|扫码参与|联系微微学姐/.test(text)) report(failures, file, "包含广告或活动信息，不是有效题目");

  for (const image of body.matchAll(/!\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)/g)) {
    const target = resolve(dirname(file), decodeURIComponent(image[1]));
    if (!existsSync(target)) report(failures, file, `图片不存在：${image[1]}`);
  }

  const sections = splitSections(body);
  const type = metadata.type;
  if (type === "single-choice") {
    const stem = sections.get("题目") ?? sections.get("共用题干") ?? "";
    if (!stem) report(failures, file, "缺少题干");
    const options = sections.get("选项") ?? "";
    const answer = sections.get("参考答案") ?? "";
    validateOptions(file, options, answer);
    const scope = metadata.chapter ? `章节:${metadata.chapter}` : `试卷:${metadata.exam ?? "未分类"}`;
    const signature = normalized(`${stem}\n${options}\n${answer}`);
    if (signature) {
      const key = `${scope}\0${signature}`;
      const group = duplicateQuestions.get(key) ?? [];
      group.push(expectedID);
      duplicateQuestions.set(key, group);
    }
  } else if (type === "compound-choice") {
    if (!(sections.get("共用题干") ?? sections.get("题目"))) report(failures, file, "缺少共用题干");
    validateCompound(file, sections);
  } else if (type === "content") {
    if (!["题目", "案例说明", "问题", "共用题干"].some((name) => sections.get(name))) {
      report(failures, file, "内容题缺少正文");
    }
    if (/选择题|上午/.test(metadata.exam ?? "")) {
      report(failures, file, "上午选择题缺少可作答的选项和参考答案");
    }
  } else {
    report(failures, file, `不支持的题型：${type ?? "空"}`);
  }
}

for (const group of duplicateQuestions.values()) {
  if (group.length > 1) failures.push(`同一章节或试卷内完全重复：${group.join(", ")}`);
}

console.log(`已检查 ${files.length} 道题：${failures.length} 个错误，${warnings.length} 个提醒`);
for (const message of failures) console.error(`ERROR ${message}`);
for (const message of warnings) console.warn(`WARN  ${message}`);
process.exitCode = failures.length === 0 ? 0 : 1;
