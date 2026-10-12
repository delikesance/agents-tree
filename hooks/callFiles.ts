const FILE_ARGS = ['file_path', 'path', 'notebook_path']

export const filesOf = (call: Record<string, unknown>) => FILE_ARGS.map(key => call[key]).filter((value): value is string => typeof value === 'string' && value !== '')
