import {
	MockWorkspaceAgent,
	MockWorkspaceAgentDevcontainer,
} from "testHelpers/entities";
import { createTestQueryClient } from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { renderHook, waitFor } from "@testing-library/react";
import * as API from "api/api";
import type { WorkspaceAgentListContainersResponse } from "api/typesGenerated";
import * as GlobalSnackbar from "components/GlobalSnackbar/utils";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { act } from "react";
import { QueryClientProvider } from "react-query";
import type { OneWayWebSocket } from "utils/OneWayWebSocket";
import { useAgentContainers } from "./useAgentContainers";

const createWrapper = (): FC<PropsWithChildren> => {
	const queryClient = createTestQueryClient();
	return ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
};

describe("useAgentContainers", () => {
	it("returns containers when agent is connected", async () => {
		server.use(
			http.get(
				`/api/v2/workspaceagents/${MockWorkspaceAgent.id}/containers`,
				() => {
					return HttpResponse.json({
						devcontainers: [MockWorkspaceAgentDevcontainer],
						containers: [],
					});
				},
			),
		);

		const { result } = renderHook(
			() => useAgentContainers(MockWorkspaceAgent),
			{
				wrapper: createWrapper(),
			},
		);

		await waitFor(() => {
			expect(result.current).toEqual([MockWorkspaceAgentDevcontainer]);
		});
	});

	it("returns undefined when agent is not connected", () => {
		const disconnectedAgent = {
			...MockWorkspaceAgent,
			status: "disconnected" as const,
		};

		const { result } = renderHook(() => useAgentContainers(disconnectedAgent), {
			wrapper: createWrapper(),
		});

		expect(result.current).toBeUndefined();
	});

	it("handles API errors gracefully", async () => {
		server.use(
			http.get(
				`/api/v2/workspaceagents/${MockWorkspaceAgent.id}/containers`,
				() => {
					return HttpResponse.error();
				},
			),
		);

		const { result } = renderHook(
			() => useAgentContainers(MockWorkspaceAgent),
			{
				wrapper: createWrapper(),
			},
		);

		await waitFor(() => {
			expect(result.current).toBeUndefined();
		});
	});

	it("handles parsing errors from WebSocket", async () => {
		const displayErrorSpy = jest.spyOn(GlobalSnackbar, "displayError");
		const watchAgentContainersSpy = jest.spyOn(API, "watchAgentContainers");

		const mockSocket = {
			addEventListener: jest.fn(),
			close: jest.fn(),
		};
		watchAgentContainersSpy.mockReturnValue(
			mockSocket as unknown as OneWayWebSocket<WorkspaceAgentListContainersResponse>,
		);

		server.use(
			http.get(
				`/api/v2/workspaceagents/${MockWorkspaceAgent.id}/containers`,
				() => {
					return HttpResponse.json({
						devcontainers: [MockWorkspaceAgentDevcontainer],
						containers: [],
					});
				},
			),
		);

		const { result, unmount } = renderHook(
			() => useAgentContainers(MockWorkspaceAgent),
			{
				wrapper: createWrapper(),
			},
		);

		// Wait for initial query to complete
		await waitFor(() => {
			expect(result.current).toEqual([MockWorkspaceAgentDevcontainer]);
		});

		// Now simulate message event with parsing error
		const messageHandler = mockSocket.addEventListener.mock.calls.find(
			(call) => call[0] === "message",
		)?.[1];

		if (messageHandler) {
			act(() => {
				messageHandler({
					parseError: new Error("Parse error"),
					parsedMessage: null,
				});
			});
		}

		await waitFor(() => {
			expect(displayErrorSpy).toHaveBeenCalledWith(
				"Failed to update containers",
				"Please try refreshing the page",
			);
		});

		unmount();
		displayErrorSpy.mockRestore();
		watchAgentContainersSpy.mockRestore();
	});

	it("handles WebSocket errors", async () => {
		const displayErrorSpy = jest.spyOn(GlobalSnackbar, "displayError");
		const watchAgentContainersSpy = jest.spyOn(API, "watchAgentContainers");

		const mockSocket = {
			addEventListener: jest.fn(),
			close: jest.fn(),
		};
		watchAgentContainersSpy.mockReturnValue(
			mockSocket as unknown as OneWayWebSocket<WorkspaceAgentListContainersResponse>,
		);

		server.use(
			http.get(
				`/api/v2/workspaceagents/${MockWorkspaceAgent.id}/containers`,
				() => {
					return HttpResponse.json({
						devcontainers: [MockWorkspaceAgentDevcontainer],
						containers: [],
					});
				},
			),
		);

		const { result, unmount } = renderHook(
			() => useAgentContainers(MockWorkspaceAgent),
			{
				wrapper: createWrapper(),
			},
		);

		// Wait for initial query to complete
		await waitFor(() => {
			expect(result.current).toEqual([MockWorkspaceAgentDevcontainer]);
		});

		// Now simulate error event
		const errorHandler = mockSocket.addEventListener.mock.calls.find(
			(call) => call[0] === "error",
		)?.[1];

		if (errorHandler) {
			act(() => {
				errorHandler(new Error("WebSocket error"));
			});
		}

		await waitFor(() => {
			expect(displayErrorSpy).toHaveBeenCalledWith(
				"Failed to load containers",
				"Please try refreshing the page",
			);
		});

		unmount();
		displayErrorSpy.mockRestore();
		watchAgentContainersSpy.mockRestore();
	});

	it("does not establish WebSocket connection when agent is not connected", () => {
		const watchAgentContainersSpy = jest.spyOn(API, "watchAgentContainers");

		const disconnectedAgent = {
			...MockWorkspaceAgent,
			status: "disconnected" as const,
		};

		const { result } = renderHook(() => useAgentContainers(disconnectedAgent), {
			wrapper: createWrapper(),
		});

		expect(watchAgentContainersSpy).not.toHaveBeenCalled();
		expect(result.current).toBeUndefined();

		watchAgentContainersSpy.mockRestore();
	});

	it("does not establish WebSocket connection when dev container feature is not enabled", async () => {
		const watchAgentContainersSpy = jest.spyOn(API, "watchAgentContainers");

		server.use(
			http.get(
				`/api/v2/workspaceagents/${MockWorkspaceAgent.id}/containers`,
				() => {
					return HttpResponse.json(
						{ message: "Dev Container feature not enabled." },
						{ status: 403 },
					);
				},
			),
		);

		const { result } = renderHook(
			() => useAgentContainers(MockWorkspaceAgent),
			{
				wrapper: createWrapper(),
			},
		);

		// Wait for the query to complete and error to be processed
		await waitFor(() => {
			expect(result.current).toBeUndefined();
		});

		expect(watchAgentContainersSpy).not.toHaveBeenCalled();

		watchAgentContainersSpy.mockRestore();
	});
});
