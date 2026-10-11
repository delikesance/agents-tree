export const UNLEARNABLE_PREFIXES = ['Async agent launched successfully', 'This session is being continued', '[Subagent hand-back]'] as const

export const isLearnable = (report: string) => !UNLEARNABLE_PREFIXES.some(prefix => report.trimStart().startsWith(prefix))
