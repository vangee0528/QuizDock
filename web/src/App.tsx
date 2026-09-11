import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, APIError } from "./api";
import { Markdown } from "./Markdown";
import { prefetchQuestionUIDs, preloadQuestionAssets, QuestionLoader } from "./question-loader";
import type {
  AnswerResult, AuthStatus, BrowserSettings, Filters, Meta, Mode, Question,
  QueueItem, SessionSnapshot, Settings, UpdateCatalog,
} from "./types";

const SESSION_KEY = "quizdock.practice.v1";
const RANDOM_QUEUE_KEY = "quizdock.practice-random-queue.v1";
const BROWSER_SETTINGS_KEY = "quizdock.browser-settings.v1";

const defaultBrowserSettings: BrowserSettings = {
  autoSubmit: false,
  autoNext: false,
  arrowKeys: true,
};

function browserSettingsFromServer(settings: Settings): BrowserSettings {
  return {
    autoSubmit: settings.auto_submit,
    autoNext: settings.auto_next,
    arrowKeys: settings.arrow_keys,
  };
}

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

function loadRandomQueue(scope: string): string[] {
  try {
    const value = JSON.parse(sessionStorage.getItem(RANDOM_QUEUE_KEY) || "null") as { scope?: string; uids?: string[] } | null;
    return value?.scope === scope && Array.isArray(value.uids) ? value.uids : [];
  } catch { return []; }
}

function saveRandomQueue(scope: string, queue: QueueItem[]): void {
  try {
    sessionStorage.setItem(RANDOM_QUEUE_KEY, JSON.stringify({ scope, uids: queue.map((item) => item.uid) }));
  } catch { /* 浏览器空间不足时仍可继续当前练习，只是不恢复随机顺序。 */ }
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

function resultClass(value: boolean | null | undefined): string {
  return value === true ? "correct" : value === false ? "wrong" : "";
}

function compactLayout(): boolean {
  return window.innerWidth <= 780;
}

export function App() {
  const [auth, setAuth] = useState<AuthStatus | null>(null);
  const [meta, setMeta] = useState<Meta | null>(null);
  const [updates, setUpdates] = useState<UpdateCatalog | null>(null);
  const [mode, setMode] = useState<Mode>("sequence");
  const [filters, setFilters] = useState<Filters>({ banks: [] });
  const [queue, setQueue] = useState<QueueItem[]>([]);
  const [index, setIndex] = useState(-1);
  const [question, setQuestion] = useState<Question | null>(null);
  const [selections, setSelections] = useState<Record<string, string[]>>({});
  const [answerResult, setAnswerResult] = useState<AnswerResult | null>(null);
  const [startedAt, setStartedAt] = useState(0);
  const [loading, setLoading] = useState(true);
  const [practiceStarting, setPracticeStarting] = useState(false);
  const [importing, setImporting] = useState(false);
  const [toast, setToast] = useState("");
  const [showSettings, setShowSettings] = useState(false);
  const [showBanks, setShowBanks] = useState(false);
  const [checkingUpdates, setCheckingUpdates] = useState(false);
  const [installingBank, setInstallingBank] = useState("");
  const [navigatorCollapsed, setNavigatorCollapsed] = useState(compactLayout);
  const legacyBrowserSettings = useRef<BrowserSettings | null>(loadJSON<BrowserSettings>(BROWSER_SETTINGS_KEY));
  const [browserSettings, setBrowserSettings] = useState<BrowserSettings>(() => ({
    ...defaultBrowserSettings,
    ...(legacyBrowserSettings.current || {}),
  }));
  const fileInput = useRef<HTMLInputElement>(null);
  const restoreStarted = useRef(false);
  const snapshot = useRef<SessionSnapshot | null>(null);
  const autoNextTimer = useRef<number | null>(null);
  const progressTimer = useRef<number | null>(null);
  const pendingProgress = useRef<object | null>(null);
  const navigationRequest = useRef(0);
  const questionLoader = useRef<QuestionLoader | null>(null);
  if (questionLoader.current === null) {
    questionLoader.current = new QuestionLoader(
      api.question,
      async (uids) => {
        const questions = (await api.questionBatch(uids)).questions;
        preloadQuestionAssets(questions);
        return questions;
      },
    );
  }

  const prefetchFollowing = useCallback((targetQueue: QueueItem[], targetIndex: number) => {
    const loader = questionLoader.current!;
    const uids = prefetchQuestionUIDs(targetQueue, targetIndex, (uid) => loader.available(uid));
    if (uids.length > 0) void loader.prefetch(uids);
  }, []);

  const flushProgress = useCallback((keepalive = false) => {
    if (progressTimer.current !== null) window.clearTimeout(progressTimer.current);
    progressTimer.current = null;
    const payload = pendingProgress.current;
    pendingProgress.current = null;
    if (payload) void api.saveProgress(payload, keepalive).catch(() => undefined);
  }, []);

  const saveProgressSoon = useCallback((payload: object) => {
    pendingProgress.current = payload;
    if (progressTimer.current !== null) window.clearTimeout(progressTimer.current);
    progressTimer.current = window.setTimeout(() => flushProgress(), 300);
  }, [flushProgress]);

  const notify = useCallback((message: string) => {
    setToast(message);
    window.setTimeout(() => setToast((current) => current === message ? "" : current), 3000);
  }, []);

  const applyMeta = useCallback(async (value: Meta) => {
    if (legacyBrowserSettings.current) {
      const legacy = legacyBrowserSettings.current;
      value.settings = await api.saveSettings({
        ...value.settings,
        auto_submit: legacy.autoSubmit,
        auto_next: legacy.autoNext,
        arrow_keys: legacy.arrowKeys,
      });
      legacyBrowserSettings.current = null;
      localStorage.removeItem(BROWSER_SETTINGS_KEY);
    }
    setBrowserSettings(browserSettingsFromServer(value.settings));
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

  const refreshMeta = useCallback(async () => applyMeta(await api.meta()), [applyMeta]);

  const checkUpdates = useCallback(async (announce = true) => {
    setCheckingUpdates(true);
    try {
      const value = await api.updates(announce);
	  setUpdates(value);
	  if (announce) {
	    const bankUpdates = value.banks.filter((bank) => bank.update_available).length;
	    if (value.application.update_available || bankUpdates) notify(`发现 ${Number(value.application.update_available) + bankUpdates} 项更新`);
	    else notify("应用和官方题库均为最新版本");
	  }
	  return value;
    } catch (error) {
	  if (announce) notify(error instanceof Error ? error.message : "检查更新失败");
	  return null;
    } finally {
	  setCheckingUpdates(false);
    }
  }, [notify]);

  const saveSnapshot = useCallback((scrollY = window.scrollY) => {
    if (!question) return;
    const value: SessionSnapshot = {
      mode,
      filters: stableFilters(filters),
      currentUid: question.uid,
      queueUids: [],
      scrollY: Math.max(0, Math.round(scrollY)),
      navigatorCollapsed,
    };
    snapshot.current = value;
    sessionStorage.setItem(SESSION_KEY, JSON.stringify(value));
  }, [filters, mode, navigatorCollapsed, question, queue]);

  const openQuestion = useCallback(async (
    targetIndex: number,
    targetQueue: QueueItem[] = queue,
    targetMode: Mode = mode,
    targetFilters: Filters = filters,
    restoreScroll?: number,
  ) => {
    if (targetIndex < 0 || targetIndex >= targetQueue.length) {
      notify(targetIndex < 0 ? "已经是第一题" : "本轮练习已完成");
      return;
    }
    if (autoNextTimer.current !== null) window.clearTimeout(autoNextTimer.current);
    const requestID = ++navigationRequest.current;
    const uid = targetQueue[targetIndex].uid;
    const cached = questionLoader.current?.cached(uid);
    setLoading(!cached);
    try {
      const next = cached ?? await questionLoader.current!.load(uid);
      if (requestID !== navigationRequest.current) return;
      setIndex(targetIndex);
      setQuestion(next);
      setSelections({});
      setAnswerResult(null);
      setStartedAt(performance.now());
      history.replaceState(null, "", practiceURL(targetMode, next.uid, targetFilters));
      const scope = scopeKey(targetMode, targetFilters);
      if (targetMode !== "random" && targetMode !== "review") {
        saveProgressSoon({
          scope_key: scope, mode: targetMode, filters: stableFilters(targetFilters),
          current_uid: next.uid, current_index: targetIndex,
        });
      }
      requestAnimationFrame(() => requestAnimationFrame(() => {
        window.scrollTo({ top: restoreScroll ?? 0, behavior: "auto" });
      }));
      prefetchFollowing(targetQueue, targetIndex);
    } catch (error) {
      if (requestID !== navigationRequest.current) return;
      notify(error instanceof Error ? error.message : "加载题目失败");
    } finally {
      if (requestID === navigationRequest.current) setLoading(false);
    }
  }, [filters, mode, notify, prefetchFollowing, queue, saveProgressSoon]);

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
    const saved = loadSession();
    const requestID = ++navigationRequest.current;
    setNavigatorCollapsed(compactLayout() ? (restore ? saved?.navigatorCollapsed ?? true : true) : false);
    setPracticeStarting(true);
    setLoading(true);
    try {
      const params = queueParams(targetMode, targetFilters, meta.settings.daily_target);
      if (resumeUid) params.set("uid", resumeUid);
      if (targetMode !== "random" && targetMode !== "review") {
        params.set("scope", scopeKey(targetMode, targetFilters));
      }
      const response = await api.startPractice(params);
      if (requestID !== navigationRequest.current) return;
      let nextQueue = response.questions;
      const practiceScope = scopeKey(targetMode, targetFilters);
      let savedRandomQueue = targetMode === "random" && restore ? loadRandomQueue(practiceScope) : [];
      if (savedRandomQueue.length === 0 && targetMode === "random" && restore) {
        savedRandomQueue = saved?.queueUids || [];
      }
      if (savedRandomQueue.length) {
        const order = new Map(savedRandomQueue.map((uid, position) => [uid, position]));
        nextQueue = [...nextQueue].sort((left, right) => (order.get(left.uid) ?? Number.MAX_SAFE_INTEGER) - (order.get(right.uid) ?? Number.MAX_SAFE_INTEGER));
      }
      if (targetMode === "random") saveRandomQueue(practiceScope, nextQueue);
      setMode(targetMode);
      setFilters(stableFilters(targetFilters));
      setQueue(nextQueue);
      if (nextQueue.length === 0 || !response.question) {
        setQuestion(null);
        setIndex(-1);
        notify("当前范围没有可练习题目");
        history.replaceState(null, "", "/");
        return;
      }
      const targetIndex = Math.max(0, nextQueue.findIndex((item) => item.uid === response.question?.uid));
      const next = response.question;
      questionLoader.current?.seed(next);
      setIndex(targetIndex);
      setQuestion(next);
      setSelections({});
      setAnswerResult(null);
      setStartedAt(performance.now());
      history.replaceState(null, "", practiceURL(targetMode, next.uid, targetFilters));
      if (targetMode !== "random" && targetMode !== "review") {
        saveProgressSoon({
          scope_key: scopeKey(targetMode, targetFilters), mode: targetMode,
          filters: stableFilters(targetFilters), current_uid: next.uid, current_index: targetIndex,
        });
      }
      requestAnimationFrame(() => requestAnimationFrame(() => {
        window.scrollTo({ top: restore ? saved?.scrollY ?? 0 : 0, behavior: "auto" });
      }));
      prefetchFollowing(nextQueue, targetIndex);
    } catch (error) {
      if (requestID !== navigationRequest.current) return;
      notify(error instanceof Error ? error.message : "加载练习失败");
    } finally {
      if (requestID === navigationRequest.current) {
        setPracticeStarting(false);
        setLoading(false);
      }
    }
  }, [filters, meta, mode, notify, prefetchFollowing, saveProgressSoon]);

  useEffect(() => {
    void api.bootstrap().then((value) => {
	  setAuth(value.auth);
	  if (value.auth.authenticated && value.meta) {
	    return applyMeta(value.meta).then(() => { void checkUpdates(false); });
	  }
	  return undefined;
    }).catch((error) => notify(error instanceof Error ? error.message : "初始化失败"))
	  .finally(() => setLoading(false));
  }, [applyMeta, checkUpdates, notify]);

  useEffect(() => {
    if (!meta || restoreStarted.current) return;
    restoreStarted.current = true;
    const route = parsePracticeURL();
    if (!route) return;
    const enabled = new Set(meta.banks.filter((bank) => bank.enabled).map((bank) => bank.id));
    const banks = route.filters.banks.filter((id) => enabled.has(id));
    const restoredFilters = { ...route.filters, banks: banks.length ? banks : [...enabled] };
    void startPractice(route.mode, restoredFilters, route.uid, true);
  }, [meta, startPractice]);

  useEffect(() => {
    saveSnapshot();
  }, [saveSnapshot]);

  useEffect(() => {
    const flush = () => flushProgress(true);
    window.addEventListener("pagehide", flush);
    return () => {
      window.removeEventListener("pagehide", flush);
      flushProgress();
    };
  }, [flushProgress]);

  useEffect(() => {
    let timer = 0;
    const onScroll = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => saveSnapshot(window.scrollY), 120);
    };
    const onPageHide = () => {
      window.clearTimeout(timer);
      saveSnapshot(window.scrollY);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("pagehide", onPageHide);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("pagehide", onPageHide);
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
      setMeta((current) => current ? { ...current, stats: result.stats } : current);
      if (result.correct && browserSettings.autoNext) {
        autoNextTimer.current = window.setTimeout(() => {
          void openQuestion(index + 1);
        }, 420);
      }
    } catch (error) {
      notify(error instanceof Error ? error.message : "提交失败");
    }
  }, [answerResult, browserSettings.autoNext, index, notify, openQuestion, question, selections, startedAt]);

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

  useEffect(() => {
    if (!showBanks && !showSettings) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => { document.body.style.overflow = previous; };
  }, [showBanks, showSettings]);

  const importBank = async (file: File | undefined) => {
    if (!file) return;
    setImporting(true);
    try {
      const result = await api.importBank(file);
      questionLoader.current?.clear();
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

  const installOfficialBank = async (slug: string) => {
	setInstallingBank(slug);
	try {
	  const result = await api.installOfficialBank(slug);
	  questionLoader.current?.clear();
	  await refreshMeta();
	  await checkUpdates(false);
	  notify(`${result.updated ? "已更新" : "已安装"} ${result.name} v${result.version}，共 ${result.questions} 题`);
	} catch (error) {
	  notify(error instanceof Error ? error.message : "安装官方题库失败");
	} finally {
	  setInstallingBank("");
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
      questionLoader.current?.clear();
      await refreshMeta();
    } catch (error) {
      notify(error instanceof Error ? error.message : "更新题库失败");
    }
  };

  const removeBank = async (id: string, name: string) => {
    if (!window.confirm(`卸载“${name}”？答题记录和错题数据会保留。`)) return;
    try {
      await api.removeBank(id);
      questionLoader.current?.clear();
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
	if (!meta?.banks.some((bank) => bank.enabled)) {
	  setMode(nextMode);
	  setQuestion(null); setQueue([]); setIndex(-1); history.replaceState(null, "", "/");
	  notify("请先安装或导入题库");
	  setShowBanks(true);
	  return;
	}
    setMode(nextMode);
    setFilters((current) => ({ banks: current.banks }));
  };

  const leavePractice = () => {
    saveSnapshot();
    flushProgress();
    navigationRequest.current += 1;
    setPracticeStarting(false);
    setQuestion(null);
    setSelections({});
    setAnswerResult(null);
    history.replaceState(null, "", "/");
    requestAnimationFrame(() => window.scrollTo({ top: 0, behavior: "auto" }));
  };

  const correctCount = useMemo(() => queue.filter((item) => item.last_correct === true).length, [queue]);
  const wrongCount = useMemo(() => queue.filter((item) => item.last_correct === false).length, [queue]);
  const toggleNavigator = useCallback(() => setNavigatorCollapsed((current) => !current), []);

  if (auth?.enabled && !auth.authenticated) {
	return <LoginScreen username={auth.username} onLogin={async (username, password) => {
	  const next = await api.login(username, password);
	  setAuth(next);
	  setLoading(true);
	  try {
	    await refreshMeta();
	    void checkUpdates(false);
	  } finally {
	    setLoading(false);
	  }
	}} />;
  }

  return (
    <div className={`app-shell ${question || practiceStarting ? "practice-active" : ""}`}>
      <aside className="sidebar">
        <div className="brand">
          <img className="brand-mark" src="/favicon.svg" alt="QuizDock 标志" />
          <div className="brand-copy">
            <strong>QuizDock</strong>
            <span>本地题库刷题工具</span>
            <div className="brand-meta">
              <span>v{meta?.version || "…"}</span>
              <i />
              {updates?.application.update_available
                ? <a href={updates.application.release_url} target="_blank" rel="noreferrer">发现 v{updates.application.latest_version}</a>
                : <button onClick={() => void checkUpdates()} disabled={checkingUpdates}>{checkingUpdates ? "检查中…" : "检查更新"}</button>}
              {auth?.enabled && <>
                <i />
                <button onClick={async () => { await api.logout(); setAuth({ ...auth, authenticated: false }); setMeta(null); }}>{auth.username} · 退出</button>
              </>}
              {!question && <button className="mobile-home-settings" onClick={() => setShowSettings(true)}>设置</button>}
            </div>
          </div>
          {question && <button className="practice-exit" onClick={leavePractice}>← 返回练习选择</button>}
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
          {mode === "chapter" && <select value={filters.chapter || ""} onChange={(event) => setFilters({ ...filters, chapter: event.target.value || undefined })}><option value="">选择章节</option>{meta?.chapters?.map((value) => <option key={value}>{value}</option>)}</select>}
          {mode === "random" && <select value={filters.tag || ""} onChange={(event) => setFilters({ ...filters, tag: event.target.value || undefined })}><option value="">选择 Tag</option>{meta?.tags?.map((value) => <option key={value}>{value}</option>)}</select>}
          {mode === "exam" && <select value={filters.exam || ""} onChange={(event) => setFilters({ ...filters, exam: event.target.value || undefined })}><option value="">选择真题试卷</option>{meta?.exams?.map((value) => <option key={value}>{value}</option>)}</select>}
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
              const updated = { ...question, starred };
              questionLoader.current?.seed(updated);
              setQuestion(updated);
            }}>{question?.starred ? "★" : "☆"}</button>
          </div>
        </header>
        <div className="progress-track"><span style={{ width: queue.length ? `${(index + 1) / queue.length * 100}%` : "0%" }} /></div>

        {!question && !loading && (
          <section className="welcome-card">
            <div className="welcome-art"><span>A</span><span>B</span><span>C</span><span>D</span></div>
            <p className="eyebrow">{meta?.banks.length ? "题库已就绪" : "从题库开始"}</p>
            <h2>{meta?.banks.length ? "选择一种模式，继续你的学习进度" : "添加题库后开始练习"}</h2>
            <p>题库与应用独立更新。所有题目、进度、收藏和错题记录都保存在本地 SQLite 数据库中。</p>
            <button className="primary" onClick={() => meta?.banks.length ? void startPractice() : setShowBanks(true)}>{meta?.banks.length ? "开始练习" : "管理题库"}</button>
          </section>
        )}

        {loading && <div className="loading-card"><span className="spinner" />正在加载…</div>}

        {question && !loading && (
          <div className="practice-layout">
            {queue.length > 0 && <ProgressNavigator
              queue={queue} index={index} collapsed={navigatorCollapsed}
              correctCount={correctCount} wrongCount={wrongCount}
              onToggle={toggleNavigator} onOpen={openQuestion}
            />}

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
          </div>
        )}
      </main>

      {showBanks && meta && <BankManager
		meta={meta} updates={updates} importing={importing} checking={checkingUpdates} installing={installingBank}
		onClose={() => setShowBanks(false)} onImport={() => fileInput.current?.click()} onToggle={toggleBankEnabled}
		onRemove={removeBank} onCheck={() => void checkUpdates()} onInstall={installOfficialBank}
	  />}
      {showSettings && meta && <SettingsDialog server={meta.settings} browser={browserSettings} onClose={() => setShowSettings(false)} onSave={async (server, browser) => {
        try {
          const saved = await api.saveSettings({
            ...server,
            auto_submit: browser.autoSubmit,
            auto_next: browser.autoNext,
            arrow_keys: browser.arrowKeys,
          });
          setMeta({ ...meta, settings: saved });
          setBrowserSettings(browserSettingsFromServer(saved));
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

const NAVIGATOR_PAGE_SIZE = 160;

const ProgressNavigator = memo(function ProgressNavigator({
  queue, index, collapsed, correctCount, wrongCount, onToggle, onOpen,
}: {
  queue: QueueItem[];
  index: number;
  collapsed: boolean;
  correctCount: number;
  wrongCount: number;
  onToggle: () => void;
  onOpen: (position: number) => void | Promise<void>;
}) {
  const currentPage = Math.floor(Math.max(0, index) / NAVIGATOR_PAGE_SIZE);
  const pageCount = Math.max(1, Math.ceil(queue.length / NAVIGATOR_PAGE_SIZE));
  const [page, setPage] = useState(currentPage);
  useEffect(() => setPage(currentPage), [currentPage]);
  useEffect(() => setPage((value) => Math.min(value, pageCount - 1)), [pageCount]);
  const start = page * NAVIGATOR_PAGE_SIZE;
  const visible = useMemo(() => queue.slice(start, start + NAVIGATOR_PAGE_SIZE), [queue, start]);

  return <section className={`navigator-panel ${collapsed ? "collapsed" : ""}`}>
    <button className="navigator-heading" onClick={onToggle}>
      <span><b>答题进度</b><small>{correctCount} 对 · {wrongCount} 错 · {queue.length - correctCount - wrongCount} 未答</small></span>
      <i>{collapsed ? "⌃" : "⌄"}</i>
    </button>
    {!collapsed && <>
      {pageCount > 1 && <div className="navigator-pagination">
        <button disabled={page === 0} aria-label="上一页题号" onClick={() => setPage((value) => value - 1)}>‹</button>
        <select aria-label="题号范围" value={page} onChange={(event) => setPage(Number(event.target.value))}>
          {Array.from({ length: pageCount }, (_, position) => {
            const rangeStart = position * NAVIGATOR_PAGE_SIZE + 1;
            const rangeEnd = Math.min((position + 1) * NAVIGATOR_PAGE_SIZE, queue.length);
            return <option key={position} value={position}>{rangeStart}–{rangeEnd}</option>;
          })}
        </select>
        <span>/ {queue.length}</span>
        <button disabled={page === pageCount - 1} aria-label="下一页题号" onClick={() => setPage((value) => value + 1)}>›</button>
      </div>}
      <div className="number-grid">{visible.map((item, offset) => {
        const position = start + offset;
        return <button key={item.uid} className={`number-button ${resultClass(item.last_correct)} ${position === index ? "current" : ""}`} title={item.title} onClick={() => void onOpen(position)}>{position + 1}</button>;
      })}</div>
    </>}
  </section>;
});

function BankManager({ meta, updates, importing, checking, installing, onClose, onImport, onToggle, onRemove, onCheck, onInstall }: {
  meta: Meta;
  updates: UpdateCatalog | null;
  importing: boolean;
  checking: boolean;
  installing: string;
  onClose: () => void;
  onImport: () => void;
  onToggle: (id: string, enabled: boolean) => Promise<void>;
  onRemove: (id: string, name: string) => Promise<void>;
	onCheck: () => void;
  onInstall: (slug: string) => Promise<void>;
}) {
  return <div className="modal" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
    <section className="dialog bank-dialog">
      <div className="dialog-title">
        <div><p className="eyebrow">题库书架</p><h2>管理题库</h2><p>在一处安装官方题库、导入本地题库包，并管理已安装内容。</p></div>
        <button className="round-button" aria-label="关闭题库管理" onClick={onClose}>×</button>
      </div>

      <div className="bank-source-grid">
        <section className="bank-source official-source">
          <div className="source-heading">
            <span className="source-icon">☁</span>
            <div><h3>官方题库</h3><p>从 QuizDock 发布页直接安装，后续可自动发现更新。</p></div>
            <button className="text-button" disabled={checking} onClick={onCheck}>{checking ? "检查中…" : "刷新"}</button>
          </div>
          <div className="source-content">
            {updates?.banks.map((bank) => <article className="bank-row official" key={bank.slug}>
              <div><h3>{bank.name}</h3><p>{bank.installed ? `已安装 v${bank.installed_version}` : "尚未安装"}{bank.latest_version ? ` · 最新 v${bank.latest_version}` : ""}</p><small>题库与 QuizDock 应用独立发布</small></div>
              <div className="bank-actions">
                {bank.install_available && (!bank.installed || bank.update_available) && <button className="primary compact" disabled={Boolean(installing)} onClick={() => void onInstall(bank.slug)}>{installing === bank.slug ? "处理中…" : bank.installed ? "更新" : "安装"}</button>}
                {!bank.install_available && <a className="text-link" href={bank.release_url} target="_blank" rel="noreferrer">发布页</a>}
              </div>
            </article>)}
            {!updates && <p className="empty-note">点击“刷新”获取可用的官方题库。</p>}
          </div>
        </section>

        <section className="bank-source local-source">
          <div className="source-heading">
            <span className="source-icon">⇧</span>
            <div><h3>本地题库</h3><p>选择由你自己保管或从其他渠道获取的 `.qbank` 文件。</p></div>
          </div>
          <div className="local-import-card">
            <span>.qbank</span>
            <p>题库会先经过结构与完整性校验，再事务化写入本地数据库。</p>
            <button className="secondary" disabled={importing} onClick={onImport}>{importing ? "正在导入…" : "选择本地文件"}</button>
          </div>
        </section>
      </div>

      <section className="installed-banks">
        <div className="section-heading"><div><p className="eyebrow">已安装</p><h3>{meta.banks.length ? `${meta.banks.length} 个题库` : "尚无题库"}</h3></div></div>
        <div className="bank-list">
          {meta.banks.map((bank) => <article className="bank-row" key={bank.id}>
            <div><h3>{bank.name}</h3><p>{bank.subject || bank.exam || "通用题库"} · {bank.question_count} 题 · v{bank.version}</p><small>{bank.id}{bank.license ? ` · ${bank.license}` : ""}</small></div>
            <div className="bank-actions">
              <label className="switch" title={bank.enabled ? "已启用" : "已停用"}><input type="checkbox" checked={bank.enabled} onChange={(event) => void onToggle(bank.id, event.target.checked)} /><i /></label>
              <button className="danger-text" onClick={() => void onRemove(bank.id, bank.name)}>卸载</button>
            </div>
          </article>)}
          {!meta.banks.length && <p className="empty-note">从上方安装官方题库，或选择一个本地题库文件。</p>}
        </div>
      </section>
    </section>
  </div>;
}

function LoginScreen({ username, onLogin }: { username: string; onLogin: (username: string, password: string) => Promise<void> }) {
  const [user, setUser] = useState(username || "admin");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  return <main className="login-screen">
	<section className="login-card">
	  <img src="/favicon.svg" alt="QuizDock 标志" />
	  <p className="eyebrow">安全访问</p><h1>登录 QuizDock</h1><p>请输入部署时配置的账号和密码。</p>
	  <form onSubmit={async (event) => {
		event.preventDefault(); setSubmitting(true); setError("");
		try { await onLogin(user, password); }
		catch (reason) { setError(reason instanceof APIError ? reason.message : "登录失败"); }
		finally { setSubmitting(false); }
	  }}>
		<label><span>用户名</span><input autoComplete="username" value={user} onChange={(event) => setUser(event.target.value)} /></label>
		<label><span>密码</span><input autoFocus type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></label>
		{error && <p className="login-error">{error}</p>}
		<button className="primary wide" disabled={submitting}>{submitting ? "正在登录…" : "登录"}</button>
	  </form>
	</section>
  </main>;
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
