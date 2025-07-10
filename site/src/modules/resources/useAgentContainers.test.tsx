import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import {
	MockWorkspaceAgent,
	MockWorkspaceAgentDevcontainer,
} from "testHelpers/entities";
import { server } from "testHelpers/server";
import { useAgentContainers } from "./useAgentContainers";

const createWrapper = (): FC<PropsWithChildren> => {
	const queryClient = new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
			},
		},
	});
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
});
