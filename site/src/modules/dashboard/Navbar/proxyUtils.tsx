import type { Proxies, ProxyLatencies } from "contexts/ProxyContext";

export function sortProxiesByLatency(
	proxies: Proxies,
	latencies: ProxyLatencies,
) {
	return [...proxies].sort((a, b) => {
		const latencyA = latencies?.[a.id]?.latencyMS ?? Number.POSITIVE_INFINITY;
		const latencyB = latencies?.[b.id]?.latencyMS ?? Number.POSITIVE_INFINITY;
		return latencyA - latencyB;
	});
}
