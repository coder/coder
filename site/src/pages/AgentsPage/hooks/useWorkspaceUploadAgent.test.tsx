import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { useWorkspaceUploadAgent } from "./useWorkspaceUploadAgent";

const createWrapper = () => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	return ({ children }: React.PropsWithChildren) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
};

const workspaceAtBuild = (
	buildNumber: number,
	agents: TypesGen.WorkspaceAgent[],
): TypesGen.Workspace => ({
	...MockWorkspace,
	latest_build: {
		...MockWorkspace.latest_build,
		id: `build-${buildNumber}`,
		build_number: buildNumber,
		resources: [{ ...MockWorkspace.latest_build.resources[0], agents }],
	},
});

// The new-chat page's list keeps this copy after a rebuild replaces the agent.
const listedWorkspace = workspaceAtBuild(1, [MockWorkspaceAgent]);
const rebuiltAgent: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "agent-rebuilt",
};
const rebuiltWorkspace = (status: TypesGen.WorkspaceAgentStatus) =>
	workspaceAtBuild(2, [{ ...rebuiltAgent, status }]);

afterEach(() => {
	vi.useRealTimers();
	vi.restoreAllMocks();
});

describe("useWorkspaceUploadAgent", () => {
	it("stays unresolved until a fresh copy holds a newer build's selected agent", async () => {
		vi.spyOn(API.experimental, "getChatWorkspaceAgent").mockResolvedValue({
			agent_id: rebuiltAgent.id,
		});
		let resolveWorkspace: (workspace: TypesGen.Workspace) => void = () => {};
		const getWorkspace = vi.spyOn(API, "getWorkspace").mockReturnValue(
			new Promise((resolve) => {
				resolveWorkspace = resolve;
			}),
		);

		const { result } = renderHook(
			() => useWorkspaceUploadAgent(listedWorkspace),
			{ wrapper: createWrapper() },
		);

		await waitFor(() =>
			expect(getWorkspace).toHaveBeenCalledWith(listedWorkspace.id),
		);
		expect(result.current).toMatchObject({
			isResolved: false,
			canUpload: false,
		});

		resolveWorkspace(rebuiltWorkspace("connected"));
		await waitFor(() =>
			expect(result.current).toMatchObject({
				isResolved: true,
				canUpload: true,
			}),
		);
	});

	it("reads a newer build again until its selected agent connects", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		vi.spyOn(API.experimental, "getChatWorkspaceAgent").mockResolvedValue({
			agent_id: rebuiltAgent.id,
		});
		let agentStatus: TypesGen.WorkspaceAgentStatus = "connecting";
		vi.spyOn(API, "getWorkspace").mockImplementation(async () =>
			rebuiltWorkspace(agentStatus),
		);

		const { result } = renderHook(
			() => useWorkspaceUploadAgent(listedWorkspace),
			{ wrapper: createWrapper() },
		);

		await waitFor(() =>
			expect(result.current).toMatchObject({
				isResolved: true,
				canUpload: false,
			}),
		);
		agentStatus = "connected";
		await act(() => vi.advanceTimersByTimeAsync(5_000));
		await waitFor(() => expect(result.current.canUpload).toBe(true));
	});

	it("reports no eligible agent without polling when a newer build has none", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		vi.spyOn(API.experimental, "getChatWorkspaceAgent").mockResolvedValue({});
		const getWorkspace = vi
			.spyOn(API, "getWorkspace")
			.mockResolvedValue(rebuiltWorkspace("connected"));

		const { result } = renderHook(
			() => useWorkspaceUploadAgent(listedWorkspace),
			{ wrapper: createWrapper() },
		);

		await waitFor(() =>
			expect(result.current).toMatchObject({
				isResolved: true,
				noEligibleAgent: true,
				canUpload: false,
			}),
		);
		const readsBeforeWaiting = getWorkspace.mock.calls.length;
		await act(() => vi.advanceTimersByTimeAsync(10_000));
		expect(getWorkspace).toHaveBeenCalledTimes(readsBeforeWaiting);
	});
});
