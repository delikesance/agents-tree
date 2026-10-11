const ULTRACODE_KEYWORD = /\bultracode\b/i

export const ULTRACODE_NOTE = [
  "Demande ultracode: orchestre-la avec l'outil Workflow, pas avec l'outil Agent ni fanout.py. Le flow parallèle et le mode conseillé du mod sont suspendus pour cette demande.",
  "Dans le script, fixe `model: 'haiku'` sur chaque agent(), quelle que soit l'étape ; n'omets jamais `model`.",
  "Ne passe à `model: 'sonnet'` que pour relancer une étape dont l'exécution Haiku a échoué (résultat nul, test en échec, réponse NEEDS WORKER), toujours avec `effort: 'low'`.",
  "Économie: repère la liste de travail toi-même avant le script, un seul workflow de 6 agents au plus, pas de panel de vérification ni de juges, `schema` avec tailles bornées sur chaque agent(), `isolation: 'worktree'` seulement pour des éditions parallèles, et aucun appel à l'advisor.",
].join('\n')

export const asksUltracode = (text: string) => ULTRACODE_KEYWORD.test(text)

export const ultracodeNote = (text: string) => (asksUltracode(text) ? ULTRACODE_NOTE : undefined)
