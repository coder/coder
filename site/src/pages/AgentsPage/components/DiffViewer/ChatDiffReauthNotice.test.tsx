import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toast } from "#/components/Toaster/toast";
import { MockChatDiffReauthRequired } from "#/testHelpers/chatEntities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	ChatDiffReauthNotice,
	POPUP_POLL_INTERVAL_MS,
} from "./ChatDiffReauthNotice";

type PopupStub = { closed: boolean };

const openPopup = (popup: PopupStub) =>
	vi.spyOn(window, "open").mockReturnValue(popup as Window);

const renderNotice = () => {
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<ChatDiffReauthNotice
				chatId="chat-1"
				reauth={MockChatDiffReauthRequired}
			/>
		</QueryClientProvider>,
	);
	return userEvent.setup();
};

const reconnectButton = () =>
	screen.getByRole("button", { name: "Reconnect GitHub" });

describe("ChatDiffReauthNotice", () => {
	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
	});

	it("opens the provider page in a popup and ignores clicks while connecting", async () => {
		const openSpy = openPopup({ closed: false });
		const user = renderNotice();

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

	it("lets the owner reconnect again after the popup closes", async () => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		const popup = { closed: false };
		const openSpy = openPopup(popup);
		const user = renderNotice();

		await user.click(reconnectButton());
		popup.closed = true;
		await act(async () => {
			await vi.advanceTimersByTimeAsync(POPUP_POLL_INTERVAL_MS);
		});

		await user.click(reconnectButton());
		expect(openSpy).toHaveBeenCalledTimes(2);
	});

	it("reports a blocked popup and stays ready to reconnect", async () => {
		const openSpy = vi.spyOn(window, "open").mockReturnValue(null);
		const toastSpy = vi.spyOn(toast, "error");
		const user = renderNotice();

		await user.click(reconnectButton());
		expect(toastSpy).toHaveBeenCalledWith(
			"Pop-up blocked. Allow pop-ups to reconnect.",
		);

		await user.click(reconnectButton());
		expect(openSpy).toHaveBeenCalledTimes(2);
	});
});
