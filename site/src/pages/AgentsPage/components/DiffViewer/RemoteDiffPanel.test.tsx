import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AxiosError, AxiosHeaders } from "axios";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { MockChatDiffReauthRequired } from "#/testHelpers/chatEntities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes from "#/theme";
import { ThemeContextProvider } from "#/theme/context";
import type { ChatMessageInputRef } from "../AgentChatInput";
import { POPUP_POLL_INTERVAL_MS } from "./ChatDiffReauthNotice";
import { RemoteDiffPanel } from "./RemoteDiffPanel";

const axiosError = (status: number, data: unknown): AxiosError =>
	new AxiosError("request failed", "ERR_BAD_RESPONSE", undefined, undefined, {
		status,
		statusText: "",
		headers: {},
		config: { headers: new AxiosHeaders() },
		data,
	});

// The tests set `closed`, which is readonly on Window.
const popupStub = (): { closed: boolean } => ({ closed: false });

const renderPanel = () => {
	const chatInputRef: React.RefObject<ChatMessageInputRef | null> = {
		current: null,
	};
	render(
		<ThemeContextProvider theme={themes.dark}>
			<QueryClientProvider client={createTestQueryClient()}>
				<TooltipProvider>
					<RemoteDiffPanel
						chatId="chat-1"
						isExpanded
						chatInputRef={chatInputRef}
						diffStyle="unified"
						remoteRef={{
							remote_origin: "https://github.com/coder/coder",
							git_branch: "feature/diff",
						}}
					/>
				</TooltipProvider>
			</QueryClientProvider>
		</ThemeContextProvider>,
	);
};

describe("RemoteDiffPanel reauth handling", () => {
	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
	});

	it("reloads the diff after the owner closes the reconnect popup", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		const diffSpy = vi
			.spyOn(API.experimental, "getChatDiffContents")
			.mockRejectedValueOnce(axiosError(424, MockChatDiffReauthRequired))
			.mockResolvedValueOnce({ chat_id: "chat-1", diff: "" });
		const popup = popupStub();
		vi.spyOn(window, "open").mockReturnValue(popup as Window);
		renderPanel();

		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", { name: "Reconnect GitHub" }),
		);
		popup.closed = true;
		await act(async () => {
			await vi.advanceTimersByTimeAsync(POPUP_POLL_INTERVAL_MS);
		});

		await waitFor(() => expect(diffSpy).toHaveBeenCalledTimes(2));
	});

	it("keeps the error for a failure that is not a reauth response", async () => {
		// The body would show a reconnect button if the guard accepted it.
		vi.spyOn(API.experimental, "getChatDiffContents").mockRejectedValue(
			axiosError(500, {
				...MockChatDiffReauthRequired,
				kind: "generic",
				message: "Failed to get chat diff.",
			}),
		);
		renderPanel();

		await screen.findByText("Failed to get chat diff.");
		expect(
			screen.queryByRole("button", { name: "Reconnect GitHub" }),
		).toBeNull();
	});
});
