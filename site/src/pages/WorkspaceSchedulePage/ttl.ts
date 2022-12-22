export interface AutoStop {
  autoStopEnabled: boolean
  ttl: number
}

export const emptyTTL = 0

const msToHours = (ms: number) => Math.round(ms / (1000 * 60 * 60))

export const ttlMsToAutoStop = (ttl_ms?: number): AutoStop =>
  ttl_ms
    ? { autoStopEnabled: true, ttl: msToHours(ttl_ms) }
    : { autoStopEnabled: false, ttl: 0 }
