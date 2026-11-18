import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chatDiffContentsFamilyKey } from "#/api/queries/chats";
import type { ChatDiffReauthRequired } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { MockChatDiffReauthRequired } from "#/testHelpers/chatEntities";
import { MockGithubExternalAuth } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	ChatDiffReauthNotice,
	POPUP_POLL_INTERVAL_MS,
} from "./ChatDiffReauthNotice";

// Only `closed` is read from the popup. Window.closed is readonly, so the stub is mutable.
type PopupStub = { closed: boolean };

const openPopup = (popup: PopupStub) =>
	vi.spyOn(window, "open").mockReturnValue(popup as Window);

const renderNotice = (
	reauth: ChatDiffReauthRequired = MockChatDiffReauthRequired,
) => {
	const queryClient = createTestQueryClient();
	const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
	render(
		<QueryClientProvider client={queryClient}>
			<ChatDiffReauthNotice chatId="chat-1" reauth={reauth} />
		</QueryClientProvider>,
	);
	return { user: userEvent.setup(), invalidateSpy };
};

const reconnectButton = () =>
	screen.getByRole("button", { name: "Reconnect GitHub" });

describe("ChatDiffReauthNotice", () => {
	beforeEach(() => {
		vi.spyOn(API, "getExternalAuthProvider").mockResolvedValue(
			MockGithubExternalAuth,
		);
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
	});

	it("opens the provider page in a popup and ignores clicks while connecting", async () => {
		const openSpy = openPopup({ closed: false });
		const { user } = renderNotice();

		await user.click(reconnectButton());
		expect(openSpy).toHaveBeenCalledWith(
			"/external-auth/github-test",
			"_blank",
			"width=900,height=600",
		);

		await user.click(
			screen.getByRole("button", { name: "Waiting for reconnect…" }),
		);
		expect(openSpy).toHaveBeenCalledTimes(1);
	});

	it("reloads the diff when the popup closes", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		const popup = { closed: false };
		const openSpy = openPopup(popup);
		const { user, invalidateSpy } = renderNotice();

		await user.click(reconnectButton());
		popup.closed = true;
		await act(async () => {
			await vi.advanceTimersByTimeAsync(POPUP_POLL_INTERVAL_MS);
		});

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({
				queryKey: chatDiffContentsFamilyKey("chat-1"),
			}),
		);
		await user.click(reconnectButton());
		expect(openSpy).toHaveBeenCalledTimes(2);
	});

	it("ends connecting and reloads the diff once the provider reports the account authenticated", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		const openSpy = openPopup({ closed: false });
		vi.spyOn(API, "getExternalAuthProvider").mockResolvedValue({
			...MockGithubExternalAuth,
			authenticated: true,
		});
		const { user, invalidateSpy } = renderNotice();

		await user.click(reconnectButton());
		await act(async () => {
			await vi.advanceTimersByTimeAsync(POPUP_POLL_INTERVAL_MS);
		});

		expect(invalidateSpy).toHaveBeenCalledWith(
			expect.objectContaining({
				queryKey: chatDiffContentsFamilyKey("chat-1"),
			}),
		);
		await user.click(reconnectButton());
		expect(openSpy).toHaveBeenCalledTimes(2);
	});

	it("reports a blocked popup and stays ready to reconnect", async () => {
		const openSpy = vi.spyOn(window, "open").mockReturnValue(null);
		const toastSpy = vi.spyOn(toast, "error");
		const { user } = renderNotice();

		await user.click(reconnectButton());
		expect(toastSpy).toHaveBeenCalledWith(
			"Pop-up blocked. Allow pop-ups to reconnect.",
		);

		await user.click(reconnectButton());
		expect(openSpy).toHaveBeenCalledTimes(2);
	});
});
