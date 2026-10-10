const FIRST_NAMES = [
  'Alice', 'Basile', 'Camille', 'Dorian', 'Élise', 'Félix', 'Gaspard', 'Hugo', 'Inès', 'Jules', 'Kenza', 'Léa', 'Malo', 'Nina', 'Oscar', 'Pauline',
  'Quentin', 'Rosalie', 'Samuel', 'Théo', 'Ulysse', 'Violette', 'Wilfried', 'Xavier', 'Yasmine', 'Zoé', 'Adèle', 'Bruno', 'Clara', 'Damien', 'Émile', 'Fanny',
]
const HASH_MULTIPLIER = 31

const hash = (text: string) => [...text].reduce((sum, char) => (sum * HASH_MULTIPLIER + char.charCodeAt(0)) >>> 0, 0)

export const agentName = (id: string) => FIRST_NAMES[hash(id) % FIRST_NAMES.length]
