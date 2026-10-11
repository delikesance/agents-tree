const ULTRACODE_KEYWORD = /\bultracode\b/i

export const ULTRACODE_NOTE = [
  "Demande ultracode: orchestre-la avec l'outil Workflow, pas avec l'outil Agent ni fanout.py. Le flow parallèle et le mode conseillé du mod sont suspendus pour cette demande.",
  "Dans le script, fixe `model` sur chaque agent(): 'haiku' pour la lecture seule (recherche, inventaire, résumé), 'sonnet' pour les éditions, les tests et la revue.",
  "N'omets `model` (il hérite alors du modèle principal) que pour une étape de conception ou d'arbitrage difficile. Mets `effort: 'low'` sur les étapes mécaniques.",
].join('\n')

export const asksUltracode = (text: string) => ULTRACODE_KEYWORD.test(text)

export const ultracodeNote = (text: string) => (asksUltracode(text) ? ULTRACODE_NOTE : undefined)
