import {
	screen,
	render as testingLibraryRender,
	waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type * as ApiModule from "#/api/api";
import { API, watchChatDesktop } from "#/api/api";
import {
	MockStoppedWorkspace,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentConnecting,
	MockWorkspaceAgentDisconnected,
	MockWorkspaceBuild,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import { DesktopPanel } from "./DesktopPanel";

vi.mock("#/api/api", async (importOriginal) => ({
	...(await importOriginal<typeof ApiModule>()),
	watchChatDesktop: vi.fn(),
}));

// The panel is driven through the real useDesktopConnection hook; only
// the noVNC client is replaced so no canvas or socket is needed.
const { rfbDisconnect } = vi.hoisted(() => ({ rfbDisconnect: vi.fn() }));
vi.mock("@novnc/novnc/lib/rfb", () => ({
	default: class FakeRFB {
		scaleViewport = false;
		resizeSession = false;
		focusOnClick = false;
		background = "";
		disconnect = rfbDisconnect;
		addEventListener = vi.fn();
	},
}));

const mockWatchChatDesktop = vi.mocked(watchChatDesktop);

// A bare query client keeps `rerender` inside the same provider so prop
// changes reach the mounted panel instead of remounting it.
const render = (element: ReactNode) => {
	const queryClient = createTestQueryClient();
	return testingLibraryRender(element, {
		wrapper: ({ children }) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		),
	});
};

describe("DesktopPanel", () => {
	beforeEach(() => {
		vi.stubGlobal("ResizeObserver", MockResizeObserver);
		mockWatchChatDesktop.mockReset();
		mockWatchChatDesktop.mockImplementation(
			() => ({ close: vi.fn() }) as unknown as WebSocket,
		);
		rfbDisconnect.mockClear();
	});

	it("starts the workspace when it is stopped", async () => {
		const startWorkspace = vi
			.spyOn(API, "startWorkspace")
			.mockResolvedValue(MockWorkspaceBuild);

		render(
			<DesktopPanel
				chatId="chat-1"
				workspace={MockStoppedWorkspace}
				workspaceAgent={MockWorkspaceAgentDisconnected}
				isVisible
			/>,
		);

		await userEvent.click(
			screen.getByRole("button", { name: /start workspace/i }),
		);

		await waitFor(() => {
			expect(startWorkspace).toHaveBeenCalledWith(
				MockStoppedWorkspace.id,
				MockStoppedWorkspace.latest_build.template_version_id,
				undefined,
				undefined,
			);
		});
		expect(mockWatchChatDesktop).not.toHaveBeenCalled();
	});

	it("dials the desktop only once the agent is connected", async () => {
		const { rerender } = render(
			<DesktopPanel
				chatId="chat-1"
				workspace={MockWorkspace}
				workspaceAgent={MockWorkspaceAgentConnecting}
				isVisible
			/>,
		);
		expect(mockWatchChatDesktop).not.toHaveBeenCalled();

		rerender(
			<DesktopPanel
				chatId="chat-1"
				workspace={MockWorkspace}
				workspaceAgent={MockWorkspaceAgent}
				isVisible
			/>,
		);
		await waitFor(() => {
			expect(mockWatchChatDesktop).toHaveBeenCalledWith("chat-1");
		});

		rerender(
			<DesktopPanel
				chatId="chat-1"
				workspace={MockStoppedWorkspace}
				workspaceAgent={MockWorkspaceAgentDisconnected}
				isVisible
			/>,
		);
		await waitFor(() => {
			expect(rfbDisconnect).toHaveBeenCalled();
		});
	});
});
