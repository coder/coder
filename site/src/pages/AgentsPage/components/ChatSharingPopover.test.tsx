import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { ChatShareButton } from "./ChatSharingPopover";

const chatId = "chat-1";

describe("ChatSharingPopover", () => {
	it("copies the absolute chat link to the clipboard", async () => {
		vi.spyOn(API.experimental, "getChatACL").mockResolvedValue({
			users: [],
			groups: [],
		});
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();

		renderWithAuth(<ChatShareButton chatId={chatId} organizationId="org-1" />);

		await user.click(await screen.findByRole("button", { name: "Share" }));
		await user.click(await screen.findByRole("button", { name: "Copy link" }));

		expect(writeText).toHaveBeenCalledWith(
			`${window.location.origin}/agents/${chatId}`,
		);
	});
});
