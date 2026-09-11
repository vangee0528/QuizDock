import type { Question } from "./types";

type LoadOne = (uid: string) => Promise<Question>;
type LoadMany = (uids: string[]) => Promise<Question[]>;

export const PREFETCH_COUNT = 5;

function markdownImageSources(markdown: string): string[] {
  const sources: string[] = [];
  const pattern = /!\[[^\]]*\]\(\s*(?:<([^>]+)>|([^\s)]+))/g;
  for (const match of markdown.matchAll(pattern)) {
    const source = match[1] || match[2];
    if (source) sources.push(source);
  }
  return sources;
}

export function questionAssetURLs(question: Question): string[] {
  if (!question.asset_base) return [];
  const markdown = [
    question.stem_md,
    ...question.parts.flatMap((part) => [part.prompt_md || "", ...part.options.map((option) => option.body_md)]),
  ];
  const urls = markdown.flatMap(markdownImageSources)
    .filter((source) => !/^(?:[a-z][a-z0-9+.-]*:|\/|#)/i.test(source))
    .map((source) => source.replace(/^(?:\.\.\/)+/, "").replace(/^\.\//, ""))
    .map((source) => `${question.asset_base}${source}`);
  return [...new Set(urls)];
}

export function preloadQuestionAssets(questions: Question[]): void {
  if (typeof Image === "undefined") return;
  for (const url of questions.flatMap(questionAssetURLs)) {
    const image = new Image();
    image.decoding = "async";
    image.src = url;
  }
}

export function prefetchQuestionUIDs(
  queue: Array<{ uid: string }>,
  currentIndex: number,
  available: (uid: string) => boolean,
  count = PREFETCH_COUNT,
): string[] {
  const upcoming = queue.slice(currentIndex + 1, currentIndex + 1 + count);
  if (upcoming.every((item) => available(item.uid))) return [];
  return queue.slice(currentIndex + 1)
    .map((item) => item.uid)
    .filter((uid) => !available(uid))
    .slice(0, count);
}

export class QuestionLoader {
  private readonly cache = new Map<string, Question>();
  private readonly inFlight = new Map<string, Promise<Question>>();

  constructor(
    private readonly loadOne: LoadOne,
    private readonly loadMany: LoadMany,
    private readonly capacity = 24,
  ) {}

  cached(uid: string): Question | undefined {
    const question = this.cache.get(uid);
    if (!question) return undefined;
    this.cache.delete(uid);
    this.cache.set(uid, question);
    return question;
  }

  seed(question: Question): void {
    this.remember(question);
  }

  available(uid: string): boolean {
    return this.cache.has(uid) || this.inFlight.has(uid);
  }

  load(uid: string): Promise<Question> {
    const cached = this.cached(uid);
    if (cached) return Promise.resolve(cached);
    const existing = this.inFlight.get(uid);
    if (existing) return existing.catch(() => this.load(uid));

    let request: Promise<Question>;
    request = this.loadOne(uid)
      .then((question) => {
        this.remember(question);
        return question;
      })
      .finally(() => {
        if (this.inFlight.get(uid) === request) this.inFlight.delete(uid);
      });
    this.inFlight.set(uid, request);
    return request;
  }

  prefetch(uids: string[]): Promise<void> {
    const missing = [...new Set(uids)].filter((uid) => !this.cache.has(uid) && !this.inFlight.has(uid));
    if (missing.length === 0) return Promise.resolve();

    const batch = this.loadMany(missing).then((questions) => {
      const byUID = new Map<string, Question>();
      for (const question of questions) {
        this.remember(question);
        byUID.set(question.uid, question);
      }
      return byUID;
    });
    for (const uid of missing) {
      let request: Promise<Question>;
      request = batch
        .then((questions) => {
          const question = questions.get(uid);
          if (!question) throw new Error(`题目 ${uid} 不存在`);
          return question;
        })
        .finally(() => {
          if (this.inFlight.get(uid) === request) this.inFlight.delete(uid);
        });
      request.catch(() => undefined);
      this.inFlight.set(uid, request);
    }
    return batch.then(() => undefined, () => undefined);
  }

  clear(): void {
    this.cache.clear();
    this.inFlight.clear();
  }

  private remember(question: Question): void {
    this.cache.delete(question.uid);
    this.cache.set(question.uid, question);
    while (this.cache.size > this.capacity) {
      const oldest = this.cache.keys().next().value as string | undefined;
      if (!oldest) break;
      this.cache.delete(oldest);
    }
  }
}
