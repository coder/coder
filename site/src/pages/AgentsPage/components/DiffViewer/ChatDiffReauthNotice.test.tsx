import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type QueryClient, QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chatDiffContentsFamilyKey } from "#/api/queries/chats";
import { externalAuthProvider } from "#/api/queries/externalAuth";
import type { ChatDiffReauthRequired } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { MockChatDiffReauthRequired } from "#/testHelpers/chatEntities";
import { MockGithubExternalAuth } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	ChatDiffReauthNotice,
	POPUP_POLL_INTERVAL_MS,
	POPUP_STATUS_POLL_INTERVAL_MS,
	POPUP_STATUS_POLL_TIMEOUT_MS,
} from "./ChatDiffReauthNotice";

// The tests set `closed`, which is readonly on Window.
type PopupStub = { closed: boolean };

const openPopup = (popup: PopupStub) =>
	vi.spyOn(window, "open").mockReturnValue(popup as Window);

const renderNotice = (
	queryClient: QueryClient = createTestQueryClient(),
	reauth: ChatDiffReauthRequired = MockChatDiffReauthRequired,
) => {
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

const statusCalls = () =>
	vi.mocked(API.getExternalAuthProvider).mock.calls.length;

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

	it("reloads the diff and reconnects again after the popup closes", async () => {
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

	it("does not end connecting on status cached before the popup opened", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		openPopup({ closed: false });
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(
			externalAuthProvider("github-test").queryKey,
			{ ...MockGithubExternalAuth, authenticated: true },
			{ updatedAt: Date.now() - 60_000 },
		);
		// Keep the refetch after the click pending, so only the cached status is read.
		vi.spyOn(API, "getExternalAuthProvider").mockReturnValue(
			new Promise(() => {}),
		);
		const { user, invalidateSpy } = renderNotice(queryClient);

		await user.click(reconnectButton());
		await act(async () => {
			await vi.advanceTimersByTimeAsync(POPUP_POLL_INTERVAL_MS);
		});

		expect(invalidateSpy).not.toHaveBeenCalled();
		expect(
			screen.getByRole("button", { name: "Waiting for reconnect…" }),
		).toHaveAttribute("aria-disabled", "true");
	});

	it("stops polling the provider status after a minute", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		openPopup({ closed: false });
		const { user } = renderNotice();

		await user.click(reconnectButton());
		const callsAtClick = statusCalls();
		await act(async () => {
			await vi.advanceTimersByTimeAsync(
				POPUP_STATUS_POLL_TIMEOUT_MS + 2 * POPUP_STATUS_POLL_INTERVAL_MS,
			);
		});
		const callsAtTimeout = statusCalls();
		expect(callsAtTimeout).toBeGreaterThan(callsAtClick);
		await act(async () => {
			await vi.advanceTimersByTimeAsync(5 * POPUP_STATUS_POLL_INTERVAL_MS);
		});

		expect(statusCalls()).toBe(callsAtTimeout);
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
