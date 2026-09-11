import { describe, expect, it, vi } from "vitest";
import { prefetchQuestionUIDs, questionAssetURLs, QuestionLoader } from "./question-loader";
import type { Question } from "./types";

function question(uid: string): Question {
  return {
    uid, bank_id: "bank", bank_name: "Bank", question_id: uid, title: uid,
    chapter: "", topic: "", exam: "", type: "single-choice", last_correct: null,
    stem_md: uid, tags: [], parts: [], starred: false, asset_base: "",
  };
}

describe("QuestionLoader", () => {
  it("批量预取后切题直接命中缓存", async () => {
    const loadOne = vi.fn(async (uid: string) => question(uid));
    const loadMany = vi.fn(async (uids: string[]) => uids.map(question));
    const loader = new QuestionLoader(loadOne, loadMany);

    await loader.prefetch(["q-2", "q-3", "q-4", "q-5", "q-6"]);
    await expect(loader.load("q-2")).resolves.toMatchObject({ uid: "q-2" });
    expect(loadMany).toHaveBeenCalledOnce();
    expect(loadOne).not.toHaveBeenCalled();
  });

  it("合并预取期间对同一道题的加载", async () => {
    let resolveBatch: ((questions: Question[]) => void) | undefined;
    const loadOne = vi.fn(async (uid: string) => question(uid));
    const loadMany = vi.fn(() => new Promise<Question[]>((resolve) => { resolveBatch = resolve; }));
    const loader = new QuestionLoader(loadOne, loadMany);

    const prefetch = loader.prefetch(["q-2"]);
    const loading = loader.load("q-2");
    resolveBatch?.([question("q-2")]);
    await prefetch;
    await expect(loading).resolves.toMatchObject({ uid: "q-2" });
    expect(loadOne).not.toHaveBeenCalled();
  });

  it("预取失败时切题自动回退到单题请求", async () => {
    const loadOne = vi.fn(async (uid: string) => question(uid));
    const loadMany = vi.fn(async () => { throw new Error("batch failed"); });
    const loader = new QuestionLoader(loadOne, loadMany);

    void loader.prefetch(["q-2"]);
    await expect(loader.load("q-2")).resolves.toMatchObject({ uid: "q-2" });
    expect(loadOne).toHaveBeenCalledWith("q-2");
  });

  it("按五题一批补充预取窗口", () => {
    const queue = Array.from({ length: 10 }, (_, index) => ({ uid: `q-${index + 1}` }));
    expect(prefetchQuestionUIDs(queue, 0, () => false)).toEqual(["q-2", "q-3", "q-4", "q-5", "q-6"]);
    const available = new Set(["q-2", "q-3", "q-4", "q-5", "q-6"]);
    expect(prefetchQuestionUIDs(queue, 1, (uid) => available.has(uid))).toEqual(["q-7", "q-8", "q-9", "q-10"]);
    available.add("q-7");
    available.add("q-8");
    expect(prefetchQuestionUIDs(queue, 2, (uid) => available.has(uid))).toEqual([]);
  });

  it("提取后续题目中的本地图片资源", () => {
    const value = question("q-image");
    value.asset_base = "/api/v1/assets/bank/";
    value.stem_md = "![题图](assets/a.png) ![外部](https://example.com/b.png)";
    value.parts = [{
      part_index: 1, label: "本题", prompt_md: "![提示](./assets/c.png)",
      options: [{ label: "A", body_md: "![选项](assets/a.png)" }],
    }];
    expect(questionAssetURLs(value)).toEqual([
      "/api/v1/assets/bank/assets/a.png",
      "/api/v1/assets/bank/assets/c.png",
    ]);
  });
});
