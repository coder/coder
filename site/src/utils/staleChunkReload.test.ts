import type * as StaleChunkReload from "./staleChunkReload";

const reload = vi.fn();
const addEventListener = vi.spyOn(window, "addEventListener");
let staleChunkReload: typeof StaleChunkReload;

const dispatchPreloadError = () => {
	window.dispatchEvent(new Event("vite:preloadError"));
};

beforeEach(async () => {
	sessionStorage.clear();
	vi.stubGlobal("location", { reload });
	// The pending flag is module state, so each test needs a fresh module.
	vi.resetModules();
	staleChunkReload = await import("./staleChunkReload");
	staleChunkReload.registerStaleChunkReload();
});

afterEach(() => {
	for (const [type, listener] of addEventListener.mock.calls) {
		window.removeEventListener(type, listener);
	}
	addEventListener.mockClear();
	vi.unstubAllGlobals();
	reload.mockReset();
});

describe("registerStaleChunkReload", () => {
	it("is not pending before a chunk fails to load", () => {
		expect(staleChunkReload.isStaleChunkReloadPending()).toBe(false);
	});

	it("reloads the page when a chunk fails to load", () => {
		dispatchPreloadError();

		expect(reload).toHaveBeenCalledTimes(1);
		expect(staleChunkReload.isStaleChunkReloadPending()).toBe(true);
	});

	it("does not reload within 10 seconds of the last reload", () => {
		sessionStorage.setItem("preload-reload", String(Date.now()));

		dispatchPreloadError();

		expect(reload).not.toHaveBeenCalled();
		expect(staleChunkReload.isStaleChunkReloadPending()).toBe(false);
	});

	it("reloads again once 10 seconds have passed", () => {
		sessionStorage.setItem("preload-reload", String(Date.now() - 11_000));

		dispatchPreloadError();

		expect(reload).toHaveBeenCalledTimes(1);
		expect(staleChunkReload.isStaleChunkReloadPending()).toBe(true);
	});
});
