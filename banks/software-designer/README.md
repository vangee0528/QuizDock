# 软件设计师题库

这是随 QuizDock 仓库维护的软件设计师题库源，当前版本为 0.1.0。应用程序不会在编译时嵌入这些内容。推送 `qbank/software-designer/v0.1.0` 形式的独立标签后，题库发布工作流会将本目录打包为 `software-designer-0.1.0.qbank`，用户可按需安装或更新。

## 构建与校验

```bash
quizdock bank pack . ../../release/software-designer-0.1.0.qbank
quizdock bank validate ../../release/software-designer-0.1.0.qbank
```

题卡已经移除采集平台名称、平台 URL、抓取时间、翻页地址、来源追溯字段、旧外部题号、`complete` 状态和带平台特征的资源名。内部题目 ID 会在后续版本中保持稳定。

## 内容许可

题库内容许可目前标记为 `Unspecified`。在确认题目及图片的再分发权利并选择正式许可前，不应对外发布题库包。QuizDock 应用代码仍独立采用 MIT 许可证。
