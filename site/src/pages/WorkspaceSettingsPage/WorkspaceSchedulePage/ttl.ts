export interface Autostop {
  autostopEnabled: boolean;
  ttl: number;
}

export const emptyTTL = 0;

const msToHours = (ms: number) => Math.round(ms / (1000 * 60 * 60));

export const ttlMsToAutostop = (ttl_ms?: number): Autostop =>
  ttl_ms
    ? { autostopEnabled: true, ttl: msToHours(ttl_ms) }
    : { autostopEnabled: false, ttl: 0 };
