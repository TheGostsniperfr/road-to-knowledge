// A plain download link: the server builds the .apkg, the browser saves it, Anki opens it.
export function AnkiExport({ scope, cards }: { scope: string; cards: number }) {
  if (cards === 0) return null;
  return (
    <a class="btn ghost" href={`/api/anki/export?scope=${encodeURIComponent(scope)}`} download>
      Exporter vers Anki · {cards} carte{cards > 1 ? "s" : ""}
    </a>
  );
}
