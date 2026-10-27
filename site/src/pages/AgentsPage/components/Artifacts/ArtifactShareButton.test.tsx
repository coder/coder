import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockChatArtifact, MockChatUser } from "#/testHelpers/chatEntities";
import {
	MockOrganizationMember2,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { ArtifactShareButton } from "./ArtifactShareButton";

describe("ArtifactShareButton", () => {
	it("copies a link to the shown version when it is not the latest", async () => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();
		render(
			<ArtifactShareButton
				artifact={MockChatArtifact}
				version={1}
				currentUserId={MockUserOwner.id}
			/>,
		);

		await user.click(
			await screen.findByRole("button", { name: "Share artifact" }),
		);
		await user.click(
			await screen.findByRole("button", { name: "Copy link to v1" }),
		);

		expect(writeText).toHaveBeenCalledWith(
			`${window.location.origin}/agents/artifacts/${MockChatArtifact.id}?version=1`,
		);
	});

	it("offers only the latest link for the latest version", async () => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();
		render(
			<ArtifactShareButton
				artifact={MockChatArtifact}
				version={MockChatArtifact.latest_version}
				currentUserId={MockUserOwner.id}
			/>,
		);

		await user.click(
			await screen.findByRole("button", { name: "Share artifact" }),
		);
		await user.click(await screen.findByRole("button", { name: "Copy link" }));

		expect(writeText).toHaveBeenCalledWith(
			`${window.location.origin}/agents/artifacts/${MockChatArtifact.id}`,
		);
	});

	const renderOwnerShare = () =>
		render(
			<ArtifactShareButton
				artifact={MockChatArtifact}
				version={MockChatArtifact.latest_version}
				currentUserId={MockUserOwner.id}
			/>,
		);

	it("shares the artifact with the picked user", async () => {
		const user = userEvent.setup();
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({ canShare: true });
		vi.spyOn(API.experimental, "getChatArtifactACL").mockResolvedValue({
			users: [],
			groups: [],
		});
		vi.spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
			members: [MockOrganizationMember2],
			count: 1,
		});
		vi.spyOn(API, "getGroupsByOrganization").mockResolvedValue([]);
		const updateACL = vi
			.spyOn(API.experimental, "updateChatArtifactACL")
			.mockResolvedValue();
		renderOwnerShare();

		await user.click(
			await screen.findByRole("button", { name: "Share artifact" }),
		);
		await user.click(
			await screen.findByRole("button", { name: "Search for user or group" }),
		);
		await user.click(
			await screen.findByRole("option", {
				name: new RegExp(MockOrganizationMember2.username),
			}),
		);
		await user.click(screen.getByRole("button", { name: "Share" }));

		await waitFor(() =>
			expect(updateACL).toHaveBeenCalledWith(MockChatArtifact.id, {
				user_roles: { [MockOrganizationMember2.user_id]: "read" },
			}),
		);
	});

	it("stops sharing with a user when the viewer may share", async () => {
		const user = userEvent.setup();
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({ canShare: true });
		vi.spyOn(API.experimental, "getChatArtifactACL").mockResolvedValue({
			users: [MockChatUser],
			groups: [],
		});
		const updateACL = vi
			.spyOn(API.experimental, "updateChatArtifactACL")
			.mockResolvedValue();
		renderOwnerShare();

		await user.click(
			await screen.findByRole("button", { name: "Share artifact" }),
		);
		await user.click(
			await screen.findByRole("button", {
				name: `Stop sharing with ${MockChatUser.username}`,
			}),
		);

		expect(updateACL).toHaveBeenCalledWith(MockChatArtifact.id, {
			user_roles: { [MockChatUser.id]: "" },
		});
	});

	it("loads no shares when the viewer may not share", async () => {
		const user = userEvent.setup();
		const checkAuthorization = vi
			.spyOn(API, "checkAuthorization")
			.mockResolvedValue({ canShare: false });
		const getACL = vi.spyOn(API.experimental, "getChatArtifactACL");
		render(
			<ArtifactShareButton
				artifact={MockChatArtifact}
				version={MockChatArtifact.latest_version}
				currentUserId={MockUserMember.id}
			/>,
		);

		await user.click(
			await screen.findByRole("button", { name: "Share artifact" }),
		);

		await waitFor(() => expect(checkAuthorization).toHaveBeenCalled());
		expect(getACL).not.toHaveBeenCalled();
	});
});
