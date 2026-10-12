import type * as StaleChunkReload from "./staleChunkReload";

const dispatchPreloadError = () => {
	window.dispatchEvent(new Event("vite:preloadError", { cancelable: true }));
};

describe("registerStaleChunkReload", () => {
	const reload = vi.fn();
	let staleChunkReload: typeof StaleChunkReload;
	let unregister: () => void;

	beforeEach(async () => {
		sessionStorage.clear();
		vi.stubGlobal("location", { reload });
		// The pending flag is module state, so each test needs a fresh module.
		vi.resetModules();
		staleChunkReload = await import("./staleChunkReload");
		unregister = staleChunkReload.registerStaleChunkReload();
	});

	afterEach(() => {
		unregister();
		vi.unstubAllGlobals();
		reload.mockReset();
	});

	it("reloads and reports the reload as pending when a chunk fails to load", () => {
		dispatchPreloadError();

		expect(reload).toHaveBeenCalledTimes(1);
		expect(staleChunkReload.isStaleChunkReloadPending()).toBe(true);
	});

	it("does not reload again within the guard window so persistent failures surface", () => {
		sessionStorage.setItem("preload-reload", String(Date.now()));

		dispatchPreloadError();

		expect(reload).not.toHaveBeenCalled();
		expect(staleChunkReload.isStaleChunkReloadPending()).toBe(false);
	});

	it("reloads again once the guard window has passed", () => {
		sessionStorage.setItem("preload-reload", String(Date.now() - 11_000));

		dispatchPreloadError();

		expect(reload).toHaveBeenCalledTimes(1);
		expect(staleChunkReload.isStaleChunkReloadPending()).toBe(true);
	});
});
