import { HttpResponse, http, ws } from "msw";
import { setupServer } from "msw/node";
import { handlers } from "./handlers";

// Tests run without a backend, so a request no handler matches would fail to
// connect anyway. Failing it here keeps MSW from opening a real connection,
// which can crash the test worker if it is still pending at teardown.
const unhandledRequests = http.all("*", ({ request }) => {
	console.warn(
		`[MSW] Warning: intercepted a request without a matching request handler:\n\n  • ${request.method} ${new URL(request.url).pathname}`,
	);
	return HttpResponse.error();
});

// Accepts every WebSocket connection that a test has not mocked and never
// sends anything on it, for the same reason as above.
const unmockedWebSockets = ws
	.link("*")
	.addEventListener("connection", () => {});

// This configures a request mocking server with the given request handlers.
export const server = setupServer(
	...handlers,
	unhandledRequests,
	unmockedWebSockets,
);
