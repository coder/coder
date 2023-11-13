export interface Autostop {
  autostopEnabled: boolean;
  ttl: number;
  ttl_bump: number;
}

export const emptyTTL = 0;

const msToHours = (ms: number) => Math.round(ms / (1000 * 60 * 60));

export const ttlMsToAutostop = (
  ttl_ms?: number,
  ttl_bump_ms?: number,
): Autostop =>
  ttl_ms
    ? {
        autostopEnabled: true,
        ttl: msToHours(ttl_ms),
        ttl_bump: msToHours(ttl_bump_ms || 0),
      }
    : { autostopEnabled: false, ttl: 0, ttl_bump: msToHours(ttl_bump_ms || 0) };
