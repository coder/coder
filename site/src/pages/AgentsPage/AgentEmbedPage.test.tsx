import { cleanup, render, waitFor } from "@testing-library/react";
import { type PropsWithChildren, useEffect } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import AgentEmbedPage from "./AgentEmbedPage";
import type { ChatSessionManager } from "./chatSession/ChatSessionManager";
import { ChatSessionsProvider } from "./chatSession/ChatSessionsProvider";
import { useChatSessionsManager } from "./chatSession/hooks";

const authContextMock = vi.hoisted(() => vi.fn());

vi.mock("#/contexts/auth/AuthProvider", () => ({
	useAuthContext: authContextMock,
}));

vi.mock("#/contexts/ProxyContext", () => ({
	ProxyProvider: ({ children }: PropsWithChildren) => children,
}));

vi.mock("#/modules/dashboard/DashboardProvider", () => ({
	DashboardProvider: ({ children }: PropsWithChildren) => children,
}));

const createPageTestQueryClient = (): QueryClient =>
	new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
				gcTime: 0,
				refetchOnWindowFocus: false,
				networkMode: "offlineFirst",
				staleTime: Number.POSITIVE_INFINITY,
			},
		},
	});

const ManagerCapture = ({
	onManager,
}: {
	onManager: (manager: ChatSessionManager) => void;
}) => {
	const manager = useChatSessionsManager();
	useEffect(() => {
		onManager(manager);
	}, [manager, onManager]);
	return null;
};

const renderEmbedPage = ({
	onEmbedManager,
	onOuterManager,
}: {
	onEmbedManager: (manager: ChatSessionManager) => void;
	onOuterManager?: (manager: ChatSessionManager) => void;
}) => {
	const queryClient = createPageTestQueryClient();
	const router = createMemoryRouter(
		[
			{
				path: "/agents/:agentId/embed",
				element: <AgentEmbedPage />,
				children: [
					{
						index: true,
						element: <ManagerCapture onManager={onEmbedManager} />,
					},
				],
			},
		],
		{ initialEntries: ["/agents/chat-a/embed?theme=light"] },
	);
	const routerElement = <RouterProvider router={router} />;
	const content = onOuterManager ? (
		<ChatSessionsProvider
			setChatErrorReason={vi.fn()}
			clearChatErrorReason={vi.fn()}
		>
			<ManagerCapture onManager={onOuterManager} />
			{routerElement}
		</ChatSessionsProvider>
	) : (
		routerElement
	);

	return render(
		<QueryClientProvider client={queryClient}>{content}</QueryClientProvider>,
	);
};

beforeEach(() => {
	authContextMock.mockReturnValue({
		isSignedIn: true,
		isSignedOut: false,
		isLoading: false,
	});
});

afterEach(() => {
	cleanup();
	vi.clearAllMocks();
	document.documentElement.classList.remove("light", "dark");
	delete document.documentElement.dataset.embedTheme;
});

describe("AgentEmbedPage chat session provider wiring", () => {
	it("provides chat sessions to the signed-in outlet", async () => {
		let embedManager: ChatSessionManager | undefined;
		renderEmbedPage({
			onEmbedManager: (manager) => {
				embedManager = manager;
			},
		});

		await waitFor(() => {
			expect(embedManager).toBeDefined();
		});
	});

	it("creates an embed manager isolated from an outer provider", async () => {
		let outerManager: ChatSessionManager | undefined;
		let embedManager: ChatSessionManager | undefined;
		renderEmbedPage({
			onOuterManager: (manager) => {
				outerManager = manager;
			},
			onEmbedManager: (manager) => {
				embedManager = manager;
			},
		});

		await waitFor(() => {
			expect(outerManager).toBeDefined();
			expect(embedManager).toBeDefined();
		});
		expect(embedManager).not.toBe(outerManager);
	});
});
