import { MockPermissions, MockUserOwner } from "testHelpers/entities";
import { renderWithRouter } from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { PropsWithChildren } from "react";
import { createMemoryRouter, type RouteObject } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("modules/dashboard/DashboardProvider", () => ({
	DashboardProvider: ({ children }: PropsWithChildren) => children,
}));

vi.mock("contexts/ProxyContext", () => ({
	ProxyProvider: ({ children }: PropsWithChildren) => children,
}));

import AgentEmbedPage from "./AgentEmbedPage";
import AgentEmbedSessionPage from "./AgentEmbedSessionPage";

const waitingForVSCodeAuthenticationText =
	/waiting for vs code authentication/i;

const postMessageMock = vi.fn();

const parentWindow = {
	postMessage: postMessageMock,
};

const originalParent = window.parent;

const createDeferred = <T,>() => {
	let resolve!: (value: T | PromiseLike<T>) => void;
	let reject!: (reason?: unknown) => void;
	const promise = new Promise<T>((res, rej) => {
		resolve = res;
		reject = rej;
	});
	return { promise, resolve, reject };
};

const setMockParentWindow = () => {
	Object.defineProperty(window, "parent", {
		configurable: true,
		value: parentWindow,
	});
};

const restoreParentWindow = () => {
	Object.defineProperty(window, "parent", {
		configurable: true,
		value: originalParent,
	});
};

const dispatchParentMessage = (
	data: unknown,
	source: MessageEventSource | null = parentWindow as unknown as Window,
) => {
	const event = new MessageEvent("message", { data });
	Object.defineProperty(event, "source", {
		configurable: true,
		value: source,
	});
	window.dispatchEvent(event);
};

const renderAgentEmbedRoutes = (route: string) => {
	const routes: RouteObject[] = [
		{
			path: "/agents/:agentId/embed/session",
			element: <AgentEmbedSessionPage />,
		},
		{
			path: "/agents/:agentId/embed",
			element: <AgentEmbedPage />,
			children: [
				{
					index: true,
					element: <div data-testid="agent-detail-sentinel">Agent detail</div>,
				},
			],
		},
	];
	const router = createMemoryRouter(routes, {
		initialEntries: [route],
	});

	return renderWithRouter(router);
};

describe("AgentEmbedSessionPage", () => {
	beforeEach(() => {
		postMessageMock.mockReset();
		setMockParentWindow();
	});

	afterEach(() => {
		restoreParentWindow();
	});

	it("sends a ready message once the page reaches the signed-out bootstrap state", async () => {
		server.use(
			http.get("/api/v2/users/me", () => {
				return HttpResponse.json({ message: "unauthorized" }, { status: 401 });
			}),
		);

		renderAgentEmbedRoutes("/agents/agent-1/embed/session");

		await waitFor(() => {
			expect(parentWindow.postMessage).toHaveBeenCalledWith(
				{ type: "coder:vscode-ready", payload: { agentId: "agent-1" } },
				"*",
			);
		});
		expect(
			screen.getByText(waitingForVSCodeAuthenticationText),
		).toBeInTheDocument();
		expect(screen.getByTestId("loader")).toBeInTheDocument();
	});

	it("ignores malformed bootstrap messages", async () => {
		let embedSessionRequests = 0;
		server.use(
			http.get("/api/v2/users/me", () => {
				return HttpResponse.json({ message: "unauthorized" }, { status: 401 });
			}),
			http.post("/api/experimental/chats/embed-session", () => {
				embedSessionRequests += 1;
				return new HttpResponse(null, { status: 204 });
			}),
		);

		renderAgentEmbedRoutes("/agents/agent-1/embed/session");
		await waitFor(() => {
			expect(parentWindow.postMessage).toHaveBeenCalledTimes(1);
		});

		dispatchParentMessage({
			type: "coder:wrong-type",
			payload: { token: "abc" },
		});
		dispatchParentMessage({ type: "coder:vscode-auth-bootstrap" });
		dispatchParentMessage({ type: "coder:vscode-auth-bootstrap", payload: {} });
		dispatchParentMessage({
			type: "coder:vscode-auth-bootstrap",
			payload: { token: 123 },
		});
		dispatchParentMessage(
			{ type: "coder:vscode-auth-bootstrap", payload: { token: "abc" } },
			{} as MessageEventSource,
		);

		await waitFor(() => {
			expect(embedSessionRequests).toBe(0);
		});
		expect(
			screen.getByText(waitingForVSCodeAuthenticationText),
		).toBeInTheDocument();
	});

	it("ignores duplicate bootstrap messages while the embed session request is in flight", async () => {
		const responseDeferred = createDeferred<void>();
		let bootstrapped = false;
		let embedSessionRequests = 0;
		server.use(
			http.get("/api/v2/users/me", () => {
				if (!bootstrapped) {
					return HttpResponse.json(
						{ message: "unauthorized" },
						{ status: 401 },
					);
				}
				return HttpResponse.json(MockUserOwner);
			}),
			http.post("/api/experimental/chats/embed-session", async () => {
				embedSessionRequests += 1;
				await responseDeferred.promise;
				bootstrapped = true;
				return new HttpResponse(null, { status: 204 });
			}),
			http.post("/api/v2/authcheck", () => {
				return HttpResponse.json(MockPermissions);
			}),
		);

		const { router } = renderAgentEmbedRoutes("/agents/agent-1/embed/session");
		await waitFor(() => {
			expect(parentWindow.postMessage).toHaveBeenCalledTimes(1);
		});

		dispatchParentMessage({
			type: "coder:vscode-auth-bootstrap",
			payload: { token: "embed-token" },
		});
		dispatchParentMessage({
			type: "coder:vscode-auth-bootstrap",
			payload: { token: "embed-token" },
		});

		await waitFor(() => {
			expect(embedSessionRequests).toBe(1);
		});

		responseDeferred.resolve();

		await screen.findByTestId("agent-detail-sentinel");
		expect(router.state.location.pathname).toBe("/agents/agent-1/embed");
	});

	it("bootstraps auth and redirects to the authenticated embed route", async () => {
		let bootstrapped = false;
		let postedToken: string | undefined;
		let meRequests = 0;
		let authcheckRequests = 0;
		server.use(
			http.get("/api/v2/users/me", () => {
				meRequests += 1;
				if (!bootstrapped) {
					return HttpResponse.json(
						{ message: "unauthorized" },
						{ status: 401 },
					);
				}
				return HttpResponse.json(MockUserOwner);
			}),
			http.post(
				"/api/experimental/chats/embed-session",
				async ({ request }) => {
					const body = (await request.json()) as { token?: string };
					postedToken = body.token;
					bootstrapped = true;
					return new HttpResponse(null, { status: 204 });
				},
			),
			http.post("/api/v2/authcheck", () => {
				authcheckRequests += 1;
				return HttpResponse.json(MockPermissions);
			}),
		);

		const { router } = renderAgentEmbedRoutes("/agents/agent-1/embed/session");
		await waitFor(() => {
			expect(parentWindow.postMessage).toHaveBeenCalledTimes(1);
		});

		dispatchParentMessage({
			type: "coder:vscode-auth-bootstrap",
			payload: { token: "embed-token" },
		});

		await screen.findByTestId("agent-detail-sentinel");

		expect(postedToken).toBe("embed-token");
		expect(meRequests).toBeGreaterThanOrEqual(2);
		expect(authcheckRequests).toBeGreaterThanOrEqual(1);
		expect(router.state.location.pathname).toBe("/agents/agent-1/embed");
	});

	it.each([400, 401, 404])(
		"shows a retryable error for a %i bootstrap failure and stays on the session route",
		async (statusCode) => {
			server.use(
				http.get("/api/v2/users/me", () => {
					return HttpResponse.json(
						{ message: "unauthorized" },
						{ status: 401 },
					);
				}),
				http.post("/api/experimental/chats/embed-session", () => {
					return HttpResponse.json(
						{ message: `bootstrap failed with ${statusCode}` },
						{ status: statusCode },
					);
				}),
			);

			const { router } = renderAgentEmbedRoutes(
				"/agents/agent-1/embed/session",
			);
			await waitFor(() => {
				expect(parentWindow.postMessage).toHaveBeenCalledTimes(1);
			});

			dispatchParentMessage({
				type: "coder:vscode-auth-bootstrap",
				payload: { token: "embed-token" },
			});

			await screen.findByText(/unable to start embedded agent/i);
			expect(router.state.location.pathname).toBe(
				"/agents/agent-1/embed/session",
			);

			await userEvent
				.setup()
				.click(screen.getByRole("button", { name: /try again/i }));

			await waitFor(() => {
				expect(parentWindow.postMessage).toHaveBeenCalledTimes(2);
			});
			expect(
				screen.getByText(waitingForVSCodeAuthenticationText),
			).toBeInTheDocument();
		},
	);

	it("redirects immediately to the authenticated embed route when already signed in", async () => {
		const { router } = renderAgentEmbedRoutes("/agents/agent-1/embed/session");

		await screen.findByTestId("agent-detail-sentinel");
		expect(router.state.location.pathname).toBe("/agents/agent-1/embed");
		expect(parentWindow.postMessage).not.toHaveBeenCalled();
	});
});

describe("AgentEmbedPage", () => {
	it("renders the authenticated outlet content when already signed in", async () => {
		const { router } = renderAgentEmbedRoutes("/agents/agent-1/embed");

		await screen.findByTestId("agent-detail-sentinel");
		expect(router.state.location.pathname).toBe("/agents/agent-1/embed");
	});

	it("shows an unauthenticated state when the embed route is visited without a session", async () => {
		server.use(
			http.get("/api/v2/users/me", () => {
				return HttpResponse.json({ message: "unauthorized" }, { status: 401 });
			}),
		);

		renderAgentEmbedRoutes("/agents/agent-1/embed");

		await screen.findByText(/not authenticated/i);
		expect(
			screen.queryByTestId("agent-detail-sentinel"),
		).not.toBeInTheDocument();
	});
});
