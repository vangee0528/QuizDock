import type {
  AnswerResult, AuthStatus, Bank, Meta, Question, QueueItem, Settings, UpdateCatalog,
} from "./types";

export class APIError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "APIError";
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !(init.body instanceof FormData)) headers.set("Content-Type", "application/json");
  const response = await fetch(path, { ...init, headers });
  const contentType = response.headers.get("content-type") || "";
  const payload = contentType.includes("application/json") ? await response.json() : null;
  if (!response.ok) throw new APIError(response.status, payload?.error || `HTTP ${response.status}`);
  return payload as T;
}

export const api = {
  authStatus: () => request<AuthStatus>("/api/v1/auth/status"),
  login: (username: string, password: string) => request<AuthStatus>("/api/v1/auth/login", {
    method: "POST", body: JSON.stringify({ username, password }),
  }),
  logout: () => request<{ logged_out: boolean }>("/api/v1/auth/logout", { method: "POST" }),
  meta: () => request<Meta>("/api/v1/meta"),
  updates: (refresh = false) => request<UpdateCatalog>(`/api/v1/updates?refresh=${refresh}`),
  banks: () => request<{ banks: Bank[] }>("/api/v1/banks"),
  importBank: (file: File) => {
    const body = new FormData();
    body.append("bank", file);
    return request<{ name: string; version: string; questions: number }>("/api/v1/banks/import", { method: "POST", body });
  },
  installOfficialBank: (slug: string) => request<{ name: string; version: string; questions: number; updated: boolean }>(
    `/api/v1/official-banks/${encodeURIComponent(slug)}/install`, { method: "POST" },
  ),
  enableBank: (id: string, enabled: boolean) => request(`/api/v1/banks/${encodeURIComponent(id)}/enabled`, {
    method: "PUT", body: JSON.stringify({ enabled }),
  }),
  removeBank: (id: string, purgeLearning = false) => request<void>(`/api/v1/banks/${encodeURIComponent(id)}?purge_learning=${purgeLearning}`, { method: "DELETE" }),
  queue: (params: URLSearchParams) => request<{ questions: QueueItem[]; count: number }>(`/api/v1/questions?${params}`),
  startPractice: (params: URLSearchParams) => request<{
    questions: QueueItem[];
    count: number;
    current_index: number;
    question: Question | null;
  }>(`/api/v1/practice/start?${params}`),
  question: (uid: string) => request<Question>(`/api/v1/questions/${encodeURIComponent(uid)}`),
  answer: (uid: string, answers: Record<string, string[]>, durationMs: number) => request<AnswerResult>("/api/v1/answers", {
    method: "POST", body: JSON.stringify({ uid, answers, duration_ms: durationMs }),
  }),
  progress: (scope: string) => request<{ progress: { current_uid: string } | null }>(`/api/v1/progress?scope=${encodeURIComponent(scope)}`),
  saveProgress: (payload: object) => request("/api/v1/progress", { method: "PUT", body: JSON.stringify(payload) }),
  setStarred: (uid: string, starred: boolean) => request(`/api/v1/questions/${encodeURIComponent(uid)}/state`, {
    method: "PUT", body: JSON.stringify({ starred }),
  }),
  saveSettings: (settings: Settings) => request<Settings>("/api/v1/settings", { method: "PUT", body: JSON.stringify(settings) }),
};
