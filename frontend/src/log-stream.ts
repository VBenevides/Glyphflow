export type LogChunk = { sequence: number; text: string }

export function mergeChunks(current: LogChunk[], incoming: LogChunk[]) {
  const bySequence = new Map(current.map((chunk) => [chunk.sequence, chunk.text]))
  let duplicates = 0
  for (const chunk of incoming) {
    if (bySequence.has(chunk.sequence)) duplicates += 1
    else bySequence.set(chunk.sequence, chunk.text)
  }
  const chunks = [...bySequence.entries()].sort(([a], [b]) => a - b).map(([sequence, text]) => ({ sequence, text }))
  const gap = chunks.some((chunk, index) => index > 0 && chunk.sequence !== chunks[index - 1].sequence + 1)
  return { chunks, duplicates, gap, lastSequence: chunks.length ? chunks[chunks.length - 1].sequence : 0 }
}
