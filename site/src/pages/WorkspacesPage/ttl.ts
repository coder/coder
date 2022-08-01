export interface AutoStop {
  enabled: boolean
  ttl: number
}

export const emptyTTL = 0

export const defaultTTL = 8

const msToHours = (ms: number) => Math.round(ms / (1000 * 60 * 60))

export const ttlMsToAutoStop = (ttl_ms?: number): AutoStop => (
  ttl_ms ? { enabled: true, ttl: msToHours(ttl_ms) } : { enabled: false, ttl: 0 }
)
