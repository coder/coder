import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockGroup } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { ChatShareButton } from "./ChatSharingPopover";

const chatId = "chat-1";

describe("ChatSharingPopover", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
	});

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

	it("renders shared group emoji avatars with their final alt text", async () => {
		// jsdom never loads images, and Radix Avatar renders no <img> until it does.
		vi.stubGlobal(
			"Image",
			class extends EventTarget {
				complete = true;
				naturalWidth = 1;
				src = "";
			},
		);
		vi.spyOn(API.experimental, "getChatACL").mockResolvedValue({
			users: [],
			groups: [
				{
					...MockGroup,
					display_name: "Design",
					avatar_url: "/emojis/1f34c.png",
					role: "read",
				},
			],
		});
		const user = userEvent.setup();

		renderWithAuth(<ChatShareButton chatId={chatId} organizationId="org-1" />);

		await user.click(await screen.findByRole("button", { name: "Share" }));
		const [row] = await screen.findAllByRole("row", { name: /Design/ });

		expect(within(row).getByRole("img")).toHaveAttribute("alt", "Banana emoji");
	});
});
