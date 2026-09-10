import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "./api";
import { Markdown } from "./Markdown";
import type {
  AnswerResult, BrowserSettings, Filters, Meta, Mode, Question,
  QuestionSummary, SessionSnapshot, Settings,
} from "./types";

const SESSION_KEY = "quizdock.practice.v1";
const BROWSER_SETTINGS_KEY = "quizdock.browser-settings.v1";

const defaultBrowserSettings: BrowserSettings = {
  autoSubmit: false,
  autoNext: false,
  arrowKeys: true,
};

const modeNames: Record<Mode, string> = {
  sequence: "顺序模式",
  chapter: "章节模式",
  random: "随机模式",
  exam: "真题模式",
  review: "今日错题",
};

function loadJSON<T>(key: string): T | null {
  try { return JSON.parse(localStorage.getItem(key) || "null") as T | null; }
  catch { return null; }
}

function loadSession(): SessionSnapshot | null {
  try { return JSON.parse(sessionStorage.getItem(SESSION_KEY) || "null") as SessionSnapshot | null; }
  catch { return null; }
}

function stableFilters(filters: Filters): Filters {
  return {
    banks: [...filters.banks].sort(),
    ...(filters.chapter ? { chapter: filters.chapter } : {}),
    ...(filters.tag ? { tag: filters.tag } : {}),
    ...(filters.exam ? { exam: filters.exam } : {}),
  };
}

function scopeKey(mode: Mode, filters: Filters): string {
  return `${mode}:${JSON.stringify(stableFilters(filters))}`;
}

function practiceURL(mode: Mode, uid: string, filters: Filters): string {
  const params = new URLSearchParams();
  for (const bank of filters.banks) params.append("bank", bank);
  if (filters.chapter) params.set("chapter", filters.chapter);
  if (filters.tag) params.set("tag", filters.tag);
  if (filters.exam) params.set("exam", filters.exam);
  return `/practice/${mode}/${encodeURIComponent(uid)}?${params}`;
}

function parsePracticeURL(): { mode: Mode; uid: string; filters: Filters } | null {
  const match = window.location.pathname.match(/^\/practice\/(sequence|chapter|random|exam|review)\/([^/]+)$/);
  if (!match) return null;
  const params = new URLSearchParams(window.location.search);
  return {
    mode: match[1] as Mode,
    uid: decodeURIComponent(match[2]),
    filters: {
      banks: params.getAll("bank"),
      chapter: params.get("chapter") || undefined,
      tag: params.get("tag") || undefined,
      exam: params.get("exam") || undefined,
    },
  };
}

function queueParams(mode: Mode, filters: Filters, dailyTarget = 20): URLSearchParams {
  const params = new URLSearchParams({ mode, limit: mode === "review" ? String(dailyTarget) : "10000" });
  for (const bank of filters.banks) params.append("bank", bank);
  if (filters.chapter) params.set("chapter", filters.chapter);
  if (filters.tag) params.set("tag", filters.tag);
  if (filters.exam) params.set("exam", filters.exam);
  return params;
}

function resultClass(value: boolean | null): string {
  return value === true ? "correct" : value === false ? "wrong" : "";
}

export function App() {
  const [meta, setMeta] = useState<Meta | null>(null);
  const [mode, setMode] = useState<Mode>("sequence");
  const [filters, setFilters] = useState<Filters>({ banks: [] });
  const [queue, setQueue] = useState<QuestionSummary[]>([]);
  const [index, setIndex] = useState(-1);
  const [question, setQuestion] = useState<Question | null>(null);
  const [selections, setSelections] = useState<Record<string, string[]>>({});
  const [answerResult, setAnswerResult] = useState<AnswerResult | null>(null);
  const [startedAt, setStartedAt] = useState(0);
  const [loading, setLoading] = useState(true);
  const [importing, setImporting] = useState(false);
  const [toast, setToast] = useState("");
  const [showSettings, setShowSettings] = useState(false);
  const [showBanks, setShowBanks] = useState(false);
  const [navigatorCollapsed, setNavigatorCollapsed] = useState(window.innerWidth < 850);
  const [browserSettings, setBrowserSettings] = useState<BrowserSettings>(() => ({
    ...defaultBrowserSettings,
    ...(loadJSON<BrowserSettings>(BROWSER_SETTINGS_KEY) || {}),
  }));
  const fileInput = useRef<HTMLInputElement>(null);
  const restoreStarted = useRef(false);
  const snapshot = useRef<SessionSnapshot | null>(null);
  const autoNextTimer = useRef<number | null>(null);

  const notify = useCallback((message: string) => {
    setToast(message);
    window.setTimeout(() => setToast((current) => current === message ? "" : current), 3000);
  }, []);

  const refreshMeta = useCallback(async () => {
    const value = await api.meta();
    setMeta(value);
    setFilters((current) => {
      const installed = value.banks.filter((bank) => bank.enabled).map((bank) => bank.id);
      if (current.banks.length === 0) return { ...current, banks: installed };
      const retained = current.banks.filter((id) => installed.includes(id));
      const newlyInstalled = installed.filter((id) => !current.banks.includes(id));
      return { ...current, banks: [...retained, ...newlyInstalled] };
    });
    return value;
  }, []);

  const saveSnapshot = useCallback((scrollY = window.scrollY) => {
    if (!question) return;
    const value: SessionSnapshot = {
      mode,
      filters: stableFilters(filters),
      currentUid: question.uid,
      queueUids: queue.map((item) => item.uid),
      scrollY: Math.max(0, Math.round(scrollY)),
      navigatorCollapsed,
    };
    snapshot.current = value;
    sessionStorage.setItem(SESSION_KEY, JSON.stringify(value));
  }, [filters, mode, navigatorCollapsed, question, queue]);

  const openQuestion = useCallback(async (
    targetIndex: number,
    targetQueue: QuestionSummary[] = queue,
    targetMode: Mode = mode,
    targetFilters: Filters = filters,
    restoreScroll?: number,
  ) => {
    if (targetIndex < 0 || targetIndex >= targetQueue.length) {
      notify(targetIndex < 0 ? "已经是第一题" : "本轮练习已完成");
      return;
    }
    if (autoNextTimer.current !== null) window.clearTimeout(autoNextTimer.current);
    setLoading(true);
    try {
      const next = await api.question(targetQueue[targetIndex].uid);
      setIndex(targetIndex);
      setQuestion(next);
      setSelections({});
      setAnswerResult(null);
      setStartedAt(performance.now());
      history.replaceState(null, "", practiceURL(targetMode, next.uid, targetFilters));
      const scope = scopeKey(targetMode, targetFilters);
      if (targetMode !== "random" && targetMode !== "review") {
        void api.saveProgress({
          scope_key: scope, mode: targetMode, filters: stableFilters(targetFilters),
          current_uid: next.uid, current_index: targetIndex,
        });
      }
      requestAnimationFrame(() => requestAnimationFrame(() => {
        window.scrollTo({ top: restoreScroll ?? 0, behavior: "auto" });
      }));
    } catch (error) {
      notify(error instanceof Error ? error.message : "加载题目失败");
    } finally {
      setLoading(false);
    }
  }, [filters, mode, notify, queue]);

  const startPractice = useCallback(async (
    targetMode: Mode = mode,
    targetFilters: Filters = filters,
    resumeUid = "",
    restore = false,
  ) => {
    if (!meta) return;
    if (targetFilters.banks.length === 0) return notify("请至少选择一个题库");
    if (targetMode === "chapter" && !targetFilters.chapter) return notify("请选择章节");
    if (targetMode === "random" && !targetFilters.tag) return notify("请选择 Tag");
    if (targetMode === "exam" && !targetFilters.exam) return notify("请选择真题试卷");
    setLoading(true);
    try {
      const response = await api.queue(queueParams(targetMode, targetFilters, meta.settings.daily_target));
      let nextQueue = response.questions;
      const saved = loadSession();
      if (targetMode === "random" && restore && saved?.queueUids.length) {
        const order = new Map(saved.queueUids.map((uid, position) => [uid, position]));
        nextQueue = [...nextQueue].sort((left, right) => (order.get(left.uid) ?? Number.MAX_SAFE_INTEGER) - (order.get(right.uid) ?? Number.MAX_SAFE_INTEGER));
      }
      setMode(targetMode);
      setFilters(stableFilters(targetFilters));
      setQueue(nextQueue);
      if (nextQueue.length === 0) {
        setQuestion(null);
        setIndex(-1);
        notify("当前范围没有可练习题目");
        history.replaceState(null, "", "/");
        return;
      }
      let uid = resumeUid;
      if (!uid && targetMode !== "random" && targetMode !== "review") {
        const savedProgress = await api.progress(scopeKey(targetMode, targetFilters));
        uid = savedProgress.progress?.current_uid || "";
      }
      const targetIndex = Math.max(0, nextQueue.findIndex((item) => item.uid === uid));
      await openQuestion(targetIndex, nextQueue, targetMode, targetFilters, restore ? saved?.scrollY : undefined);
    } catch (error) {
      notify(error instanceof Error ? error.message : "加载练习失败");
    } finally {
      setLoading(false);
    }
  }, [filters, meta, mode, notify, openQuestion]);

  useEffect(() => {
    void refreshMeta().catch((error) => notify(error instanceof Error ? error.message : "初始化失败"))
      .finally(() => setLoading(false));
  }, [notify, refreshMeta]);

  useEffect(() => {
    if (!meta || restoreStarted.current) return;
    restoreStarted.current = true;
    const route = parsePracticeURL();
    if (!route) return;
    const enabled = new Set(meta.banks.filter((bank) => bank.enabled).map((bank) => bank.id));
    const banks = route.filters.banks.filter((id) => enabled.has(id));
    const restoredFilters = { ...route.filters, banks: banks.length ? banks : [...enabled] };
    setNavigatorCollapsed(loadSession()?.navigatorCollapsed ?? window.innerWidth < 850);
    void startPractice(route.mode, restoredFilters, route.uid, true);
  }, [meta, startPractice]);

  useEffect(() => {
    saveSnapshot();
  }, [saveSnapshot]);

  useEffect(() => {
    let timer = 0;
    const onScroll = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => saveSnapshot(window.scrollY), 120);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("pagehide", onScroll);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("pagehide", onScroll);
    };
  }, [saveSnapshot]);

  const submitAnswer = useCallback(async (answers = selections) => {
    if (!question || answerResult) return;
    if (question.parts.some((part) => !answers[String(part.part_index)]?.length)) {
      notify(question.parts.length > 1 ? "请先完成每一空" : "请先选择答案");
      return;
    }
    try {
      const result = await api.answer(question.uid, answers, Math.round(performance.now() - startedAt));
      setAnswerResult(result);
      setQueue((current) => current.map((item, position) => position === index ? { ...item, last_correct: result.correct } : item));
      void refreshMeta();
      if (result.correct && browserSettings.autoNext) {
        autoNextTimer.current = window.setTimeout(() => {
          void openQuestion(index + 1);
        }, 420);
      }
    } catch (error) {
      notify(error instanceof Error ? error.message : "提交失败");
    }
  }, [answerResult, browserSettings.autoNext, index, notify, openQuestion, question, refreshMeta, selections, startedAt]);

  const selectOption = useCallback((part: number, label: string) => {
    if (!question || answerResult) return;
    const next = { ...selections, [String(part)]: [label] };
    setSelections(next);
    if (browserSettings.autoSubmit && question.parts.every((item) => next[String(item.part_index)]?.length)) {
      void submitAnswer(next);
    }
  }, [answerResult, browserSettings.autoSubmit, question, selections, submitAnswer]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (showSettings || showBanks || !question) return;
      if (browserSettings.arrowKeys && event.key === "ArrowLeft") {
        event.preventDefault(); void openQuestion(index - 1);
      } else if (browserSettings.arrowKeys && event.key === "ArrowRight") {
        event.preventDefault(); void openQuestion(index + 1);
      } else if (event.key === "Enter") {
        event.preventDefault();
        if (answerResult) void openQuestion(index + 1);
        else void submitAnswer();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [answerResult, browserSettings.arrowKeys, index, openQuestion, question, showBanks, showSettings, submitAnswer]);

  const importBank = async (file: File | undefined) => {
    if (!file) return;
    setImporting(true);
    try {
      const result = await api.importBank(file);
      await refreshMeta();
      notify(`已导入 ${result.name} ${result.version}，共 ${result.questions} 题`);
      setShowBanks(true);
    } catch (error) {
      notify(error instanceof Error ? error.message : "导入失败");
    } finally {
      setImporting(false);
      if (fileInput.current) fileInput.current.value = "";
    }
  };

  const toggleBankSelection = (id: string) => {
    setFilters((current) => ({
      ...current,
      banks: current.banks.includes(id) ? current.banks.filter((bank) => bank !== id) : [...current.banks, id],
    }));
  };

  const toggleBankEnabled = async (id: string, enabled: boolean) => {
    try {
      await api.enableBank(id, enabled);
      await refreshMeta();
    } catch (error) {
      notify(error instanceof Error ? error.message : "更新题库失败");
    }
  };

  const removeBank = async (id: string, name: string) => {
    if (!window.confirm(`卸载“${name}”？答题记录和错题数据会保留。`)) return;
    try {
      await api.removeBank(id);
      await refreshMeta();
      if (question?.bank_id === id) {
        setQuestion(null); setQueue([]); setIndex(-1); history.replaceState(null, "", "/");
      }
      notify("题库已卸载，学习记录已保留");
    } catch (error) {
      notify(error instanceof Error ? error.message : "卸载失败");
    }
  };

  const askAI = async () => {
    if (!question || !meta) return;
    const partText = question.parts.map((part) => {
      const options = part.options.map((option) => `${option.label}. ${option.body_md}`).join("\n");
      return `${question.parts.length > 1 ? part.label : "选项"}\n${part.prompt_md || ""}\n${options}\n我的作答：${selections[String(part.part_index)]?.join("、") || "未作答"}`;
    }).join("\n\n");
    const prompt = `请讲解下面这道题：先给出正确答案，再解释各选项，并总结相关知识点。\n\n题目\n${question.stem_md}\n\n${partText}`;
    try {
      await navigator.clipboard.writeText(prompt);
      window.open(meta.settings.ai_url, "_blank", "noopener,noreferrer");
      notify("题目已复制，请在 AI 页面中粘贴");
    } catch {
      notify("浏览器未允许复制，请检查剪贴板权限");
    }
  };

  const setModeAndClearFilters = (nextMode: Mode) => {
    setMode(nextMode);
    setFilters((current) => ({ banks: current.banks }));
  };

  const correctCount = useMemo(() => queue.filter((item) => item.last_correct === true).length, [queue]);
  const wrongCount = useMemo(() => queue.filter((item) => item.last_correct === false).length, [queue]);

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">Q</div>
          <div><strong>QuizDock</strong><span>Local question banks</span></div>
        </div>

        <section className="side-section overview">
          <p className="eyebrow">学习概览</p>
          <div className="stat-grid">
            <div><b>{meta?.stats.total ?? "—"}</b><span>可练题</span></div>
            <div><b>{meta?.stats.attempted ?? "—"}</b><span>已练</span></div>
            <div><b>{meta?.stats.mastered ?? "—"}</b><span>掌握</span></div>
            <div><b>{meta ? `${meta.stats.accuracy.toFixed(1)}%` : "—"}</b><span>正确率</span></div>
          </div>
        </section>

        <section className="side-section bank-selector">
          <div className="section-heading"><p className="eyebrow">当前题库</p><button className="text-button" onClick={() => setShowBanks(true)}>管理</button></div>
          {meta?.banks.length ? meta.banks.filter((bank) => bank.enabled).map((bank) => (
            <label className="bank-check" key={bank.id}>
              <input type="checkbox" checked={filters.banks.includes(bank.id)} onChange={() => toggleBankSelection(bank.id)} />
              <span><b>{bank.name}</b><small>{bank.question_count} 题 · v{bank.version}</small></span>
            </label>
          )) : <p className="muted">尚未导入题库</p>}
          <button className="secondary wide" onClick={() => fileInput.current?.click()} disabled={importing}>{importing ? "正在导入…" : "＋ 导入 .qbank"}</button>
          <input ref={fileInput} className="hidden" type="file" accept=".qbank,application/zip" onChange={(event) => void importBank(event.target.files?.[0])} />
        </section>

        <section className="side-section mode-section">
          <div className="section-heading"><p className="eyebrow">练习模式</p><span className="count-pill">{queue.length || meta?.stats.total || 0}</span></div>
          <div className="mode-tabs">
            {(["sequence", "chapter", "random", "exam"] as Mode[]).map((item) => (
              <button key={item} className={`mode-tab ${mode === item ? "active" : ""}`} onClick={() => setModeAndClearFilters(item)}>
                <b>{item === "sequence" ? "①" : item === "chapter" ? "≡" : item === "random" ? "∿" : "▣"}</b>
                <span>{modeNames[item]}<small>{item === "sequence" ? "总题库持续推进" : item === "chapter" ? "按知识章节练习" : item === "random" ? "按 Tag 随机抽题" : "按整套真题练习"}</small></span>
              </button>
            ))}
          </div>
          {mode === "chapter" && <select value={filters.chapter || ""} onChange={(event) => setFilters({ ...filters, chapter: event.target.value || undefined })}><option value="">选择章节</option>{meta?.chapters.map((value) => <option key={value}>{value}</option>)}</select>}
          {mode === "random" && <select value={filters.tag || ""} onChange={(event) => setFilters({ ...filters, tag: event.target.value || undefined })}><option value="">选择 Tag</option>{meta?.tags.map((value) => <option key={value}>{value}</option>)}</select>}
          {mode === "exam" && <select value={filters.exam || ""} onChange={(event) => setFilters({ ...filters, exam: event.target.value || undefined })}><option value="">选择真题试卷</option>{meta?.exams.map((value) => <option key={value}>{value}</option>)}</select>}
          <button className="primary wide" onClick={() => void startPractice()} disabled={loading || !meta?.banks.length}>继续练习</button>
        </section>

        <section className="side-section review-box">
          <div><p className="eyebrow">今日错题计划</p><p><b>{meta?.stats.due_reviews || 0}</b> 题待复习</p></div>
          <button className="review-button" onClick={() => void startPractice("review")} disabled={!meta?.stats.due_reviews}>开始复习</button>
        </section>
      </aside>

      <main className="workspace">
        <header className="topbar">
          <div><p className="eyebrow">{question ? `${modeNames[mode]} · ${question.bank_name}` : "可导入题库的本地刷题工具"}</p><h1>{question?.title || "QuizDock"}</h1></div>
          <div className="session-tools">
            {queue.length > 0 && <span>{index + 1} / {queue.length}</span>}
            <button className="round-button" title="练习设置" onClick={() => setShowSettings(true)}>⚙</button>
            <button className={`star-button ${question?.starred ? "active" : ""}`} disabled={!question} onClick={async () => {
              if (!question) return;
              const starred = !question.starred;
              await api.setStarred(question.uid, starred);
              setQuestion({ ...question, starred });
            }}>{question?.starred ? "★" : "☆"}</button>
          </div>
        </header>
        <div className="progress-track"><span style={{ width: queue.length ? `${(index + 1) / queue.length * 100}%` : "0%" }} /></div>

        {!question && !loading && (
          <section className="welcome-card">
            <div className="welcome-art"><span>A</span><span>B</span><span>C</span><span>D</span></div>
            <p className="eyebrow">{meta?.banks.length ? "题库已就绪" : "从题库包开始"}</p>
            <h2>{meta?.banks.length ? "选择一种模式，继续你的学习进度" : "导入一个 .qbank 题库开始练习"}</h2>
            <p>题库与应用独立更新。所有题目、进度、收藏和错题记录都保存在本地 SQLite 数据库中。</p>
            <button className="primary" onClick={() => meta?.banks.length ? void startPractice() : fileInput.current?.click()}>{meta?.banks.length ? "开始练习" : "选择题库包"}</button>
          </section>
        )}

        {loading && <div className="loading-card"><span className="spinner" />正在加载…</div>}

        {question && !loading && (
          <article className="question-card">
            <div className="tag-row">{question.tags.slice(0, 8).map((tag) => <span className="tag" key={tag}>{tag}</span>)}</div>
            <div className="markdown stem"><Markdown assetBase={question.asset_base}>{question.stem_md}</Markdown></div>
            <div className="parts">
              {question.parts.map((part) => (
                <section className="part" key={part.part_index}>
                  {question.parts.length > 1 && <h3>{part.label}</h3>}
                  {part.prompt_md && <div className="markdown"><Markdown assetBase={question.asset_base}>{part.prompt_md}</Markdown></div>}
                  <div className="option-list">
                    {part.options.map((option) => {
                      const selected = selections[String(part.part_index)]?.includes(option.label);
                      const correct = answerResult?.correct_answers[String(part.part_index)]?.includes(option.label);
                      const wrong = Boolean(answerResult && selected && !correct);
                      return (
                        <button key={option.label} disabled={Boolean(answerResult)} className={`option ${selected ? "selected" : ""} ${correct ? "correct" : ""} ${wrong ? "wrong" : ""}`} onClick={() => selectOption(part.part_index, option.label)}>
                          <span className="option-key">{option.label}</span>
                          <div className="option-body markdown"><Markdown assetBase={question.asset_base}>{option.body_md}</Markdown></div>
                        </button>
                      );
                    })}
                  </div>
                </section>
              ))}
            </div>

            {answerResult && (
              <section className={`result-panel ${answerResult.correct ? "" : "incorrect"}`}>
                <div className="result-icon">{answerResult.correct ? "✓" : "×"}</div>
                <div>
                  <h3>{answerResult.correct ? "回答正确" : answerResult.mastery ? "已加入错题计划" : "回答错误"}</h3>
                  <div className="correct-answer">{Object.entries(answerResult.correct_options).map(([part, options]) => (
                    <div key={part}>{question.parts.length > 1 ? `${question.parts[Number(part) - 1]?.label}：` : "正确答案："}{options.map((option) => <span key={option.label}><b>{option.label}.</b> {option.body_md}</span>)}</div>
                  ))}</div>
                  {!answerResult.correct && <button className="ask-ai-button" onClick={() => void askAI()}>一键问 AI ↗</button>}
                </div>
              </section>
            )}

            <footer className="card-actions">
              <span className="shortcut-hint">点击选项作答 · Enter 提交/下一题</span>
              <div>
                {!answerResult && <button className="secondary" onClick={() => void openQuestion(index + 1)}>跳过</button>}
                {!answerResult && !browserSettings.autoSubmit && <button className="primary" onClick={() => void submitAnswer()}>提交答案</button>}
                {answerResult && <button className="primary" onClick={() => void openQuestion(index + 1)}>下一题 →</button>}
              </div>
            </footer>
          </article>
        )}
      </main>

      {queue.length > 0 && (
        <section className={`navigator-panel ${navigatorCollapsed ? "collapsed" : ""}`}>
          <button className="navigator-heading" onClick={() => setNavigatorCollapsed(!navigatorCollapsed)}>
            <span><b>答题进度</b><small>{correctCount} 对 · {wrongCount} 错 · {queue.length - correctCount - wrongCount} 未答</small></span>
            <i>{navigatorCollapsed ? "⌃" : "⌄"}</i>
          </button>
          {!navigatorCollapsed && <div className="number-grid">{queue.map((item, position) => (
            <button key={item.uid} className={`number-button ${resultClass(item.last_correct)} ${position === index ? "current" : ""}`} title={item.title} onClick={() => void openQuestion(position)}>{position + 1}</button>
          ))}</div>}
        </section>
      )}

      {showBanks && meta && <BankManager meta={meta} importing={importing} onClose={() => setShowBanks(false)} onImport={() => fileInput.current?.click()} onToggle={toggleBankEnabled} onRemove={removeBank} />}
      {showSettings && meta && <SettingsDialog server={meta.settings} browser={browserSettings} onClose={() => setShowSettings(false)} onSave={async (server, browser) => {
        try {
          const saved = await api.saveSettings(server);
          setMeta({ ...meta, settings: saved });
          setBrowserSettings(browser);
          localStorage.setItem(BROWSER_SETTINGS_KEY, JSON.stringify(browser));
          setShowSettings(false);
          notify("设置已保存");
        } catch (error) {
          notify(error instanceof Error ? error.message : "保存设置失败");
        }
      }} />}
      {toast && <div className="toast">{toast}</div>}
    </div>
  );
}

function BankManager({ meta, importing, onClose, onImport, onToggle, onRemove }: {
  meta: Meta;
  importing: boolean;
  onClose: () => void;
  onImport: () => void;
  onToggle: (id: string, enabled: boolean) => Promise<void>;
  onRemove: (id: string, name: string) => Promise<void>;
}) {
  return <div className="modal" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
    <section className="dialog bank-dialog">
      <div className="dialog-title"><div><p className="eyebrow">题库书架</p><h2>管理题库</h2></div><button className="round-button" onClick={onClose}>×</button></div>
      <div className="bank-list">
        {meta.banks.map((bank) => <article className="bank-row" key={bank.id}>
          <div><h3>{bank.name}</h3><p>{bank.subject || bank.exam || "通用题库"} · {bank.question_count} 题 · v{bank.version}</p><small>{bank.id}{bank.license ? ` · ${bank.license}` : ""}</small></div>
          <div className="bank-actions">
            <label className="switch"><input type="checkbox" checked={bank.enabled} onChange={(event) => void onToggle(bank.id, event.target.checked)} /><i /></label>
            <button className="danger-text" onClick={() => void onRemove(bank.id, bank.name)}>卸载</button>
          </div>
        </article>)}
        {!meta.banks.length && <p className="empty-note">尚未导入题库。</p>}
      </div>
      <button className="primary wide" disabled={importing} onClick={onImport}>{importing ? "正在导入…" : "导入 .qbank"}</button>
    </section>
  </div>;
}

function SettingsDialog({ server, browser, onClose, onSave }: {
  server: Settings;
  browser: BrowserSettings;
  onClose: () => void;
  onSave: (server: Settings, browser: BrowserSettings) => Promise<void>;
}) {
  const [serverDraft, setServerDraft] = useState(server);
  const [browserDraft, setBrowserDraft] = useState(browser);
  return <div className="modal" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
    <section className="dialog settings-dialog">
      <div className="dialog-title"><div><p className="eyebrow">个人偏好</p><h2>练习设置</h2></div><button className="round-button" onClick={onClose}>×</button></div>
      <Toggle label="点击选项自动提交" detail="组合题在完成所有小问后提交" checked={browserDraft.autoSubmit} onChange={(value) => setBrowserDraft({ ...browserDraft, autoSubmit: value })} />
      <Toggle label="答对自动下一题" detail="短暂显示结果后快速切题" checked={browserDraft.autoNext} onChange={(value) => setBrowserDraft({ ...browserDraft, autoNext: value })} />
      <Toggle label="左右键切换题目" detail="← 上一题，→ 下一题" checked={browserDraft.arrowKeys} onChange={(value) => setBrowserDraft({ ...browserDraft, arrowKeys: value })} />
      <Toggle label="错题自动记录" detail="记录错误次数并加入每日复习" checked={serverDraft.track_wrong} onChange={(value) => setServerDraft({ ...serverDraft, track_wrong: value })} />
      <div className="plan-fields">
        <label><span>每日错题目标</span><input type="number" min="1" max="500" value={serverDraft.daily_target} onChange={(event) => setServerDraft({ ...serverDraft, daily_target: Number(event.target.value) })} /></label>
        <label><span>掌握所需连续答对</span><input type="number" min="1" max="20" value={serverDraft.required_streak} onChange={(event) => setServerDraft({ ...serverDraft, required_streak: Number(event.target.value) })} /></label>
      </div>
      <label className="field"><span><b>一键问 AI 地址</b><small>答错后复制题目并打开该网站</small></span><input type="url" value={serverDraft.ai_url} onChange={(event) => setServerDraft({ ...serverDraft, ai_url: event.target.value })} /></label>
      <button className="primary wide" onClick={() => void onSave(serverDraft, browserDraft)}>保存设置</button>
    </section>
  </div>;
}

function Toggle({ label, detail, checked, onChange }: { label: string; detail: string; checked: boolean; onChange: (value: boolean) => void }) {
  return <label className="toggle-row"><span><b>{label}</b><small>{detail}</small></span><span className="switch"><input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} /><i /></span></label>;
}
