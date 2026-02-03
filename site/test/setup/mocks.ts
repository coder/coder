// Mock useProxyLatency to avoid real network requests to external proxy URLs.
// Must use useMemo to return stable object references and prevent infinite re-renders.
vi.mock("contexts/useProxyLatency", async () => {
	const { useMemo } = await import("react");
	return {
		useProxyLatency: () => {
			const proxyLatencies = useMemo(() => ({}), []);
			return { proxyLatencies, refetch: () => new Date() };
		},
	};
});
