import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockGroup } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { ChatShareButton } from "./ChatSharingPopover";

const chatId = "chat-1";

describe("ChatSharingPopover", () => {
	afterEach(() => vi.unstubAllGlobals());

	it("adds and removes a group identified by its informative avatar", async () => {
		// Radix waits for image loading, which jsdom does not implement.
		vi.stubGlobal(
			"Image",
			class extends EventTarget {
				complete = true;
				naturalWidth = 1;
				src = "";
			},
		);
		const mockGroup = {
			...MockGroup,
			display_name: "Design",
			avatar_url: "/emojis/1f34c.png",
		};
		const getACL = vi
			.spyOn(API.experimental, "getChatACL")
			.mockResolvedValue({ users: [], groups: [] });
		vi.spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
			members: [],
			count: 0,
		});
		vi.spyOn(API, "getGroupsByOrganization").mockResolvedValue([mockGroup]);
		const updateACL = vi
			.spyOn(API.experimental, "updateChatACL")
			.mockResolvedValue();
		const user = userEvent.setup();
		renderWithAuth(<ChatShareButton chatId={chatId} organizationId="org-1" />);

		await user.click(await screen.findByRole("button", { name: "Share" }));
		await user.click(
			await screen.findByRole("button", { name: "Search for user or group" }),
		);
		await user.click(
			await screen.findByRole("option", { name: /Banana emoji Design/ }),
		);
		getACL.mockResolvedValue({
			users: [],
			groups: [{ ...mockGroup, role: "read" }],
		});
		await user.click(screen.getByRole("button", { name: "Add member" }));
		await waitFor(() =>
			expect(updateACL).toHaveBeenCalledWith(chatId, {
				group_roles: { [mockGroup.id]: "read" },
			}),
		);

		const row = await screen.findByRole("row", { name: /Banana emoji Design/ });
		await user.click(within(row).getByRole("button", { name: "Open menu" }));
		await user.click(await screen.findByRole("menuitem", { name: "Remove" }));
		await waitFor(() =>
			expect(updateACL).toHaveBeenLastCalledWith(chatId, {
				group_roles: { [mockGroup.id]: "" },
			}),
		);
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
});
