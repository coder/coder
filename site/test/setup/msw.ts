import { server } from "testHelpers/server";
import { cleanup } from "@testing-library/react";

// MSW server lifecycle
beforeAll(() => server.listen({ onUnhandledRequest: "warn" }));
afterEach(() => {
	cleanup();
	server.resetHandlers();
	vi.clearAllMocks();
});
afterAll(() => server.close());
