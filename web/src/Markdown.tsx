import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

interface Props {
  children: string;
  assetBase?: string;
}

function assetURL(source: string | undefined, assetBase: string): string | undefined {
  if (!source || /^(?:[a-z][a-z0-9+.-]*:|\/|#)/i.test(source)) return source;
  const normalized = source.replace(/^(?:\.\.\/)+/, "").replace(/^\.\//, "");
  return `${assetBase}${normalized}`;
}

export function Markdown({ children, assetBase = "" }: Props) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        a: ({ href, children: body }) => <a href={href} target="_blank" rel="noreferrer">{body}</a>,
        img: ({ src, alt }) => <img src={assetURL(src, assetBase)} alt={alt || "题目图片"} loading="lazy" />,
      }}
    >
      {children}
    </ReactMarkdown>
  );
}
