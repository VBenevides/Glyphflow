function pad(value: number) {
  return String(value).padStart(2, '0')
}

export function formatDateTime(value?: string): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : `${date.getUTCFullYear()}-${pad(date.getUTCMonth() + 1)}-${pad(date.getUTCDate())} ${pad(date.getUTCHours())}:${pad(date.getUTCMinutes())} UTC`
}

export function normalizeUtcDateTimeFilter(value: string): string {
  const match = /^(\d{4}-\d{2}-\d{2}) (\d{2}):(\d{2}) UTC$/i.exec(value.trim())
  return match ? `${match[1]}T${match[2]}:${match[3]}:00Z` : value
}
