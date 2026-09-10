import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Markdown } from "./Markdown";

describe("Markdown", () => {
  it("renders GFM tables", () => {
    render(<Markdown>{"| A | B |\n| --- | --- |\n| 1 | 2 |"}</Markdown>);
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
  });

  it("maps relative assets to the current bank endpoint", () => {
    render(<Markdown assetBase="/api/v1/assets/demo/">{"![图](../assets/example.webp)"}</Markdown>);
    expect(screen.getByRole("img")).toHaveAttribute("src", "/api/v1/assets/demo/assets/example.webp");
  });
});
