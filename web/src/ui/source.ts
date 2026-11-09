import type { Source } from "../api";

// Book notions carry their pages in the heading, "## Le journal (p. 41-58)", so a planet can open the PDF there.
export function pageOf(planet: string | null): number | null {
  const m = planet?.match(/\bp\.\s*(\d+)/i);
  return m ? Number(m[1]) : null;
}

export function sourceLabel(src: Source, page: number | null): string {
  const by = [src.author, src.year].filter(Boolean).join(", ");
  return `${src.title}${by ? " · " + by : ""}${page ? `, p. ${page}` : src.pages ? ` · ${src.pages} pages` : ""}`;
}

// Only library paths reach the browser; the server serves library/ read-only.
export function sourceHref(src: Source, page: number | null): string | null {
  if (src.available && src.file?.startsWith("library/")) return "/" + src.file.split("/").map(encodeURIComponent).join("/") + (page ? `#page=${page}` : "");
  if (src.url && /^https?:\/\//.test(src.url)) return src.url;
  return null;
}
