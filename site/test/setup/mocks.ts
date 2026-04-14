// Mock useProxyLatency to avoid real network requests to external proxy URLs.
// Must use useMemo to return stable object references and prevent infinite re-renders.
vi.mock("#/contexts/useProxyLatency", async () => {
	const { useMemo } = await import("react");
	return {
		useProxyLatency: () => {
			const proxyLatencies = useMemo(() => ({}), []);
			return { proxyLatencies, refetch: () => new Date() };
		},
	};
});

// Monaco editor relies on canvas and other browser APIs absent from JSDOM.
// Mock the modules so no real Monaco code runs in unit tests.
vi.mock("monaco-editor");
vi.mock("@monaco-editor/react", () => {
	const FakeEditor = () => null;
	return {
		default: FakeEditor,
		Editor: FakeEditor,
		DiffEditor: () => null,
		loader: { config: () => {}, init: () => Promise.resolve() },
		useMonaco: () => null,
	};
});
