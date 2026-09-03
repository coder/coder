export const unpricedRequestsMessage = (count: number): string =>
	`Cost is unavailable for ${count.toLocaleString("en-US")} ${count === 1 ? "request" : "requests"}. The total excludes that usage.`;
