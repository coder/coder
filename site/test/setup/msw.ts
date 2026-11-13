import { cleanup } from "@testing-library/react";
import { toast } from "#/components/Toaster/toast";
import { server } from "#/testHelpers/server";

// MSW server lifecycle
beforeAll(() => server.listen({ onUnhandledRequest: "warn" }));
afterEach(() => {
	cleanup();
	// Sonner's toast store is module state, and a newly mounted Toaster
	// replays every active toast, so clear them before the next test.
	toast.dismiss();
	server.resetHandlers();
	vi.clearAllMocks();
});
afterAll(async () => {
	// A leftover fake clock would make the timer flush below hang.
	if (vi.isFakeTimers()) {
		vi.useRealTimers();
	}
	// Radix FocusScope defers its unmount dispatch with setTimeout(0). Flush
	// it before vitest tears down the jsdom environment, where it would throw
	// an unhandled "parameter 1 is not of type 'Event'" TypeError.
	await new Promise((resolve) => setTimeout(resolve, 0));
	server.close();
});
