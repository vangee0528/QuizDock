import { memo, useMemo } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

interface Props {
  children: string;
  assetBase?: string;
}

const markdownPlugins = [remarkGfm];

function assetURL(source: string | undefined, assetBase: string): string | undefined {
  if (!source || /^(?:[a-z][a-z0-9+.-]*:|\/|#)/i.test(source)) return source;
  const normalized = source.replace(/^(?:\.\.\/)+/, "").replace(/^\.\//, "");
  return `${assetBase}${normalized}`;
}

export const Markdown = memo(function Markdown({ children, assetBase = "" }: Props) {
  const components = useMemo(() => ({
    a: ({ href, children: body }: React.ComponentPropsWithoutRef<"a">) => <a href={href} target="_blank" rel="noreferrer">{body}</a>,
    img: ({ src, alt }: React.ComponentPropsWithoutRef<"img">) => <img src={assetURL(src, assetBase)} alt={alt || "题目图片"} loading="lazy" />,
  }), [assetBase]);
  return useMemo(() => (
    <ReactMarkdown
      remarkPlugins={markdownPlugins}
      components={components}
    >
      {children}
    </ReactMarkdown>
  ), [children, components]);
});
