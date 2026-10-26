import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
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
		expect(
			screen.queryByRole("button", { name: /^Copy link to v/ }),
		).toBeNull();
	});

	it.each([
		[
			"the viewer's own artifact",
			{ ...MockChatArtifact },
			MockUserOwner.id,
			/Only you can open this link/,
		],
		[
			"another user's artifact",
			{ ...MockChatArtifact },
			MockUserMember.id,
			`Only ${MockUserOwner.username} can open this link.`,
		],
		[
			"a project artifact",
			{ ...MockChatArtifact, project_id: "project-1", project_name: "Billing" },
			MockUserOwner.id,
			"Members of Billing can open this link.",
		],
	])("says who can open %s", async (_, artifact, currentUserId, access) => {
		const user = userEvent.setup();
		render(
			<ArtifactShareButton
				artifact={artifact}
				version={artifact.latest_version}
				currentUserId={currentUserId}
			/>,
		);

		await user.click(
			await screen.findByRole("button", { name: "Share artifact" }),
		);

		expect(await screen.findByRole("dialog")).toHaveTextContent(access);
	});
});
