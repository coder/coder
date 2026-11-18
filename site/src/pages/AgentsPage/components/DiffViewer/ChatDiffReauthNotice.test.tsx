import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChatDiffReauthRequired } from "#/api/typesGenerated";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { ChatDiffReauthNotice } from "./ChatDiffReauthNotice";

const reauth: ChatDiffReauthRequired = {
	message: "Reconnect GitHub to load this diff.",
	code: "external_auth_reauth_required",
	provider_id: "github-test",
	provider_display_name: "GitHub",
	can_reconnect: true,
};

const renderNotice = (props: ChatDiffReauthRequired) => {
	const queryClient = createTestQueryClient();
	const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
	render(
		<QueryClientProvider client={queryClient}>
			<ChatDiffReauthNotice chatId="chat-1" reauth={props} />
		</QueryClientProvider>,
	);
	return { user: userEvent.setup(), invalidateSpy };
};

describe("ChatDiffReauthNotice", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("shows no reconnect action to a viewer who is not the chat owner", () => {
		renderNotice({ ...reauth, can_reconnect: false });

		expect(
			screen.getByText(
				"The chat owner must reconnect GitHub to load this diff.",
			),
		).toBeInTheDocument();
		expect(screen.queryByRole("button")).not.toBeInTheDocument();
	});

	it("opens the provider page in a popup and reloads the diff when it closes", async () => {
		const popup = { closed: false };
		const openSpy = vi
			.spyOn(window, "open")
			.mockReturnValue(popup as unknown as Window);
		const { user, invalidateSpy } = renderNotice(reauth);

		await user.click(screen.getByRole("button", { name: "Reconnect GitHub" }));

		expect(openSpy).toHaveBeenCalledWith(
			"/external-auth/github-test",
			"_blank",
			"width=900,height=600",
		);
		expect(
			screen.getByRole("button", { name: "Waiting for reconnect…" }),
		).toBeDisabled();

		popup.closed = true;
		await waitFor(() =>
			expect(invalidateSpy).toHaveBeenCalledWith(
				expect.objectContaining({
					queryKey: expect.arrayContaining(["diff-contents"]),
				}),
			),
		);
		expect(
			screen.getByRole("button", { name: "Reconnect GitHub" }),
		).toBeEnabled();
	});

	it("stays idle when the browser blocks the popup", async () => {
		vi.spyOn(window, "open").mockReturnValue(null);
		const { user } = renderNotice(reauth);

		await user.click(screen.getByRole("button", { name: "Reconnect GitHub" }));

		expect(
			screen.getByRole("button", { name: "Reconnect GitHub" }),
		).toBeEnabled();
	});
});
