import { render, screen } from "@testing-library/react";
import { AxiosError, AxiosHeaders } from "axios";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatDiffReauthRequired } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes from "#/theme";
import { ThemeContextProvider } from "#/theme/context";
import type { ChatMessageInputRef } from "../AgentChatInput";
import { RemoteDiffPanel } from "./RemoteDiffPanel";

const axiosError = (status: number, data: unknown): AxiosError =>
	new AxiosError("request failed", "ERR_BAD_RESPONSE", undefined, undefined, {
		status,
		statusText: "",
		headers: {},
		config: { headers: new AxiosHeaders() },
		data,
	});

const reauth: ChatDiffReauthRequired = {
	message: "Reconnect GitHub to load this diff.",
	code: "external_auth_reauth_required",
	provider_id: "github-test",
	provider_display_name: "GitHub",
	can_reconnect: true,
};

const renderPanel = (error: unknown) => {
	vi.spyOn(API.experimental, "getChatDiffContents").mockRejectedValue(error);
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
		vi.restoreAllMocks();
	});

	it("offers a reconnect to the chat owner when the provider rejected the token", async () => {
		renderPanel(axiosError(424, reauth));

		expect(
			await screen.findByRole("button", { name: "Reconnect GitHub" }),
		).toBeInTheDocument();
	});

	it("asks a viewer to have the owner reconnect", async () => {
		renderPanel(axiosError(424, { ...reauth, can_reconnect: false }));

		expect(
			await screen.findByText(
				"The chat owner must reconnect GitHub to load this diff.",
			),
		).toBeInTheDocument();
		expect(screen.queryByRole("button")).not.toBeInTheDocument();
	});

	it("keeps the error alert for other failures", async () => {
		renderPanel(axiosError(500, { message: "Failed to get chat diff." }));

		expect(
			await screen.findByText("Failed to get chat diff."),
		).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: /Reconnect/ })).toBeNull();
	});
});
