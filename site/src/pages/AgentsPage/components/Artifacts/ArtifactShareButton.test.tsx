import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MockChatArtifact } from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
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
});
