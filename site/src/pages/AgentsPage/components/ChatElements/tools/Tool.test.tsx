import { render } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { createMockWebSocket } from "#/testHelpers/websockets";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { Tool } from "./Tool";

const CHAT_BUILD_ID = "bound-build-id";

afterEach(() => {
	vi.restoreAllMocks();
});

describe("Tool workspace lifecycle rows", () => {
	it("streams build logs for a running stop_workspace call", () => {
		const watchBuildLogs = vi
			.spyOn(apiModule, "watchBuildLogsByBuildId")
			.mockImplementation(() => createMockWebSocket("ws://test")[0]);

		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<ChatWorkspaceContext value={{ buildId: CHAT_BUILD_ID }}>
					<Tool name="stop_workspace" status="running" />
				</ChatWorkspaceContext>
			</QueryClientProvider>,
		);

		expect(watchBuildLogs).toHaveBeenCalledWith(
			CHAT_BUILD_ID,
			expect.anything(),
		);
	});
});
