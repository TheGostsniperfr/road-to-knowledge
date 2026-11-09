import DOMPurify from "dompurify";
import { marked } from "marked";

const esc = (s: string) => s.replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]!);
const cardLine = /^\s*(?:[-*]\s+)?(.+?)\s+::\s+(.+?)\s*$/;
const fence = /^\s*(```|~~~)/;

// Card lines "question :: answer" become small inline cards, as in RemNote.
function cardsToHtml(md: string): string {
  let inFence = false;
  return md
    .split("\n")
    .map((line) => {
      if (fence.test(line)) inFence = !inFence;
      if (inFence) return line;
      const m = cardLine.exec(line);
      return m ? `<div class="qa"><b>${esc(m[1])}</b><span>${esc(m[2])}</span></div>\n` : line;
    })
    .join("\n");
}

// Images point at the knowledge repo's assets/ folder, which the server exposes at /assets/.
function fixAssets(html: string): string {
  return html.replace(/(src|href)="(?!https?:|\/|#|data:)([^"]*?)assets\/([^"]+)"/g, '$1="/assets/$3"');
}

export function renderMarkdown(md: string): string {
  const html = marked.parse(cardsToHtml(md), { async: false, gfm: true }) as string;
  return DOMPurify.sanitize(fixAssets(html), { ADD_ATTR: ["target"] });
}

/** Renders ```mermaid blocks lazily: the library is only downloaded when a fiche uses it. */
export async function renderMermaid(root: HTMLElement) {
  const blocks = root.querySelectorAll<HTMLElement>("pre > code.language-mermaid");
  if (!blocks.length) return;
  const { default: mermaid } = await import("mermaid");
  // SVG text labels instead of HTML ones: the sanitizer strips foreignObject, which would leave empty boxes.
  mermaid.initialize({
    startOnLoad: false, theme: "base", securityLevel: "strict", fontFamily: "Montserrat",
    flowchart: { htmlLabels: false }, htmlLabels: false,
    themeVariables: { darkMode: true, background: "#0b101b", primaryColor: "#141c2e", primaryTextColor: "#e6ecf8", primaryBorderColor: "#2fd1c1", lineColor: "#8090aa", secondaryColor: "#1d1424", tertiaryColor: "#0e1524" },
  });
  let i = 0;
  for (const code of blocks) {
    const pre = code.parentElement!;
    try {
      const { svg } = await mermaid.render(`mmd-${Date.now()}-${i++}`, code.textContent ?? "");
      const div = document.createElement("div");
      div.className = "mermaid";
      div.innerHTML = DOMPurify.sanitize(svg, { USE_PROFILES: { svg: true, svgFilters: true } });
      pre.replaceWith(div);
    } catch (err) {
      console.warn("mermaid: diagram not rendered", err);
    }
  }
}

/** Extracts the "## name" section of a fiche, for a planet. */
export function section(md: string, name: string): string {
  const lines = md.split("\n");
  let inFence = false, start = -1;
  for (let i = 0; i < lines.length; i++) {
    if (fence.test(lines[i])) inFence = !inFence;
    if (inFence || !lines[i].startsWith("## ")) continue;
    if (start >= 0) return lines.slice(start, i).join("\n");
    if (lines[i].slice(3).trim().toLowerCase() === name.toLowerCase()) start = i + 1;
  }
  return start >= 0 ? lines.slice(start).join("\n") : "";
}
