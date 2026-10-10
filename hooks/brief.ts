export const BRIEF_KEY = 'briefEnabled'
export const BRIEF_HEADER = "Pistes d'orientation (à vérifier vite, sinon cherche normalement):"
export const BRIEF_PROMPT = `Demande d'un utilisateur à un agent de code. Donne au plus 3 puces très courtes: où chercher en premier (dossiers ou fichiers seulement s'ils se déduisent de la demande), critère pour considérer la tâche finie, limite de recherche. N'invente aucun nom de fichier. Réponds uniquement par les puces.\n\n`
export const BRIEF_MAX_HITS = 3
export const BRIEF_MAX_TOKENS = 200
export const BRIEF_TIMEOUT_MS = 3000
const VAGUE_MAX_CHARS = 200
const SPECIFIC_PATTERN = /[\w./-]+\.\w{1,5}\b|[A-Za-z]+_[A-Za-z_]+|[a-z]+[A-Z]\w+/
export const isVague = (text: string) => text.length < VAGUE_MAX_CHARS && !SPECIFIC_PATTERN.test(text)

export const withTimeout = <T,>(work: Promise<T>, ms: number) =>
  Promise.race([work, new Promise<undefined>(resolve => setTimeout(resolve, ms))])
