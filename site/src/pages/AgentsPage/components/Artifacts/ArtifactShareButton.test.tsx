import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatUser } from "#/api/typesGenerated";
import { MockChatArtifact } from "#/testHelpers/chatEntities";
import { MockUserMember, MockUserOwner } from "#/testHelpers/entities";
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

	const mockSharedUser: ChatUser = {
		id: MockUserMember.id,
		username: MockUserMember.username,
		name: MockUserMember.name,
		avatar_url: MockUserMember.avatar_url,
		role: "read",
	};

	it("stops sharing with a user when the viewer may share", async () => {
		const user = userEvent.setup();
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({ canShare: true });
		vi.spyOn(API.experimental, "getChatArtifactACL").mockResolvedValue({
			users: [mockSharedUser],
			groups: [],
		});
		const updateACL = vi
			.spyOn(API.experimental, "updateChatArtifactACL")
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
		await user.click(
			await screen.findByRole("button", {
				name: `Stop sharing with ${MockUserMember.username}`,
			}),
		);

		expect(updateACL).toHaveBeenCalledWith(MockChatArtifact.id, {
			user_roles: { [MockUserMember.id]: "" },
		});
	});

	it("shows no sharing controls when the viewer may not share", async () => {
		const user = userEvent.setup();
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({ canShare: false });
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

		expect(
			await screen.findByText(
				`${MockChatArtifact.owner_username} decides who can open this link.`,
			),
		).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "Share" })).toBeNull();
		expect(getACL).not.toHaveBeenCalled();
	});
});
