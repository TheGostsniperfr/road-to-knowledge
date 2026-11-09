import { createContext } from "preact";
import { useContext } from "preact/hooks";
import type { ViewName } from "./prefs";

const plural = (n: number, one: string, many: string) => `${n} ${n > 1 ? many : one}`;

// The galaxy speaks of stars and fog; the atlas uses the plain words a colleague expects.
const galaxy = {
  root: "Univers",
  rootTitle: "Ton univers",
  theme: "Galaxie",
  summary: (known: number, fog: number) => `${plural(known, "étoile explorée", "étoiles explorées")}, ${fog} dans le brouillard`,
  counts: (clusters: number, galaxies: number) => `${plural(clusters, "amas", "amas")}, ${plural(galaxies, "galaxie", "galaxies")}.`,
  strongest: "Tes étoiles les plus brillantes",
  hint: "Clique une galaxie pour y plonger.",
  byLevel: "Étoiles par maîtrise",
  fog: "brouillard",
  links: "Trous de ver",
  traverse: "traverser",
  reviewOne: "Réviser cette étoile",
  oneInScope: "Une seule étoile dans ce périmètre.",
  allExplored: "Rien en attente. Le brouillard est dissipé.",
  noTheme: "Aucune galaxie. Ajoute un dossier dans galaxies/ ou lance la skill seed.",
};

export type Terms = typeof galaxy;

const atlas: Terms = {
  root: "Vue d'ensemble",
  rootTitle: "Vue d'ensemble",
  theme: "Thème",
  summary: (known, fog) => `${plural(known, "sujet connu", "sujets connus")}, ${fog} à explorer`,
  counts: (clusters, galaxies) => `${plural(clusters, "sous-thème", "sous-thèmes")}, ${plural(galaxies, "thème", "thèmes")}.`,
  strongest: "Tes sujets les plus solides",
  hint: "Clique un thème pour l'ouvrir.",
  byLevel: "Sujets par niveau",
  fog: "à explorer",
  links: "Liens",
  traverse: "ouvrir",
  reviewOne: "Réviser ce sujet",
  oneInScope: "Un seul sujet dans ce périmètre.",
  allExplored: "Rien en attente. Tout est exploré.",
  noTheme: "Aucun thème. Ajoute un dossier dans galaxies/ ou lance la skill seed.",
};

export const termsFor = (v: ViewName): Terms => (v === "atlas" ? atlas : galaxy);
export const TermsContext = createContext<{ view: ViewName; t: Terms }>({ view: "galaxy", t: galaxy });
export const useTerms = () => useContext(TermsContext);
