export type Mode = "sequence" | "chapter" | "random" | "exam" | "review";

export interface Bank {
  id: string;
  name: string;
  version: string;
  locale: string;
  exam: string;
  level: string;
  subject: string;
  license: string;
  description: string;
  question_count: number;
  installed: boolean;
  enabled: boolean;
  imported_at: string;
  updated_at: string;
}

export interface Settings {
  track_wrong: boolean;
  daily_target: number;
  required_streak: number;
  ai_url: string;
  auto_submit: boolean;
  auto_next: boolean;
  arrow_keys: boolean;
}

export interface Meta {
  version: string;
  banks: Bank[];
  chapters: string[];
  tags: string[];
  exams: string[];
  stats: {
    total: number;
    attempted: number;
    mastered: number;
    accuracy: number;
    due_reviews: number;
  };
  settings: Settings;
}

export interface AuthStatus {
  enabled: boolean;
  authenticated: boolean;
  username: string;
}

export interface ComponentUpdate {
  current_version: string;
  latest_version: string;
  update_available: boolean;
  release_url: string;
}

export interface BankUpdate {
  slug: string;
  id: string;
  name: string;
  installed: boolean;
  installed_version?: string;
  latest_version?: string;
  update_available: boolean;
  install_available: boolean;
  release_url: string;
  asset_url?: string;
  asset_size?: number;
}

export interface UpdateCatalog {
  checked_at: string;
  repository_url: string;
  application: ComponentUpdate;
  banks: BankUpdate[];
}

export interface QuestionSummary {
  uid: string;
  bank_id: string;
  bank_name: string;
  question_id: string;
  title: string;
  chapter: string;
  topic: string;
  exam: string;
  type: string;
  last_correct: boolean | null;
}

export interface QueueItem {
  uid: string;
  title: string;
  last_correct?: boolean | null;
}

export interface Option {
  label: string;
  body_md: string;
}

export interface Part {
  part_index: number;
  label: string;
  prompt_md?: string;
  options: Option[];
}

export interface Question extends QuestionSummary {
  stem_md: string;
  explanation_md?: string;
  tags: string[];
  parts: Part[];
  starred: boolean;
  asset_base: string;
}

export interface AnswerResult {
  correct: boolean;
  correct_answers: Record<string, string[]>;
  correct_options: Record<string, Option[]>;
  mastery?: {
    wrong_count: number;
    correct_count: number;
    consecutive_correct: number;
    status: string;
    due_date: string;
  };
}

export interface Filters {
  banks: string[];
  chapter?: string;
  tag?: string;
  exam?: string;
}

export interface BrowserSettings {
  autoSubmit: boolean;
  autoNext: boolean;
  arrowKeys: boolean;
}

export interface SessionSnapshot {
  mode: Mode;
  filters: Filters;
  currentUid: string;
  queueUids: string[];
  scrollY: number;
  navigatorCollapsed: boolean;
}
