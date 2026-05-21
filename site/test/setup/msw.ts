import { cleanup } from "@testing-library/react";
import { server } from "#/testHelpers/server";

// MSW server lifecycle
beforeAll(() => server.listen({ onUnhandledRequest: "warn" }));
afterEach(() => {
	cleanup();
	server.resetHandlers();
	vi.clearAllMocks();
});
afterAll(() => server.close());
