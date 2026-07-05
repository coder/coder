import { cleanup } from "@testing-library/react";
import { server } from "#/testHelpers/server";

// MSW server lifecycle
beforeAll(() => server.listen({ onUnhandledRequest: "warn" }));
afterEach(() => {
	cleanup();
	server.resetHandlers();
	vi.clearAllMocks();
});
afterAll(async () => {
	// If a test file left fake timers installed, restore real timers so the
	// flush below cannot hang until the hook timeout.
	if (vi.isFakeTimers()) {
		vi.useRealTimers();
	}
	// Radix's FocusScope defers its unmount event dispatch and focus restore
	// with setTimeout(0) (react#17894 workaround). The cleanup() call in
	// afterEach schedules that timer, and for the last test in a file it
	// would otherwise still be pending when vitest tears down the jsdom
	// environment. It then fires with Node's CustomEvent instead of jsdom's,
	// and jsdom rejects the dispatch with "parameter 1 is not of type
	// 'Event'" as an unhandled error. Same-delay timers run in FIFO order,
	// so awaiting one timer turn here deterministically drains every 0ms
	// timer scheduled by cleanup() while the environment is still alive.
	await new Promise((resolve) => setTimeout(resolve, 0));
	server.close();
});
