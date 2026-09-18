import MarkdownIt from "markdown-it";

const md = new MarkdownIt({ html: false, linkify: true, breaks: true });

export function renderMarkdown(src: string): string {
  return md.render(src || "");
}
