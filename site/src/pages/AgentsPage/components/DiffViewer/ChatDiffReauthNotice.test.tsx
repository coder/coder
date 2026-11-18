import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AxiosError, AxiosHeaders } from "axios";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChatDiffReauthRequired } from "#/api/typesGenerated";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { ChatDiffReauthNotice } from "./ChatDiffReauthNotice";
import { getChatDiffReauthRequired } from "./chatDiffReauth";

const reauth: ChatDiffReauthRequired = {
	message: "Reconnect GitHub to load this diff.",
	code: "external_auth_reauth_required",
	provider_id: "github-test",
	provider_display_name: "GitHub",
	can_reconnect: true,
};

const axiosError = (status: number, data: unknown): AxiosError =>
	new AxiosError("request failed", "ERR_BAD_RESPONSE", undefined, undefined, {
		status,
		statusText: "",
		headers: {},
		config: { headers: new AxiosHeaders() },
		data,
	});

describe("getChatDiffReauthRequired", () => {
	it("returns the body for a 424 with the reauth code", () => {
		expect(getChatDiffReauthRequired(axiosError(424, reauth))).toEqual(reauth);
	});

	it("ignores other statuses, even with the same code", () => {
		expect(getChatDiffReauthRequired(axiosError(500, reauth))).toBeUndefined();
	});

	it("ignores a 424 without the reauth code", () => {
		expect(
			getChatDiffReauthRequired(axiosError(424, { message: "other" })),
		).toBeUndefined();
	});

	it("ignores non-axios errors", () => {
		expect(
			getChatDiffReauthRequired(new Error("Failed to get chat diff.")),
		).toBeUndefined();
	});
});

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
