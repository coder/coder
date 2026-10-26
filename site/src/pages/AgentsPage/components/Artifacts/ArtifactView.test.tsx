import { screen } from "@testing-library/react";
import type { ChatArtifact } from "#/api/typesGenerated";
import { MockChatArtifact } from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { ArtifactView } from "./ArtifactView";

const renderView = (artifact: ChatArtifact, selectedVersion?: number) =>
	render(
		<ArtifactView
			artifact={artifact}
			selectedVersion={selectedVersion}
			onSelectVersion={() => undefined}
			currentUserId={MockUserOwner.id}
			showChatLink
		/>,
	);

describe("ArtifactView chat link", () => {
	it("links the chat of the latest version, whichever version shows", async () => {
		renderView(MockChatArtifact, 1);
		expect(
			await screen.findByRole("link", { name: "Open chat" }),
		).toHaveAttribute("href", "/agents/chat-1");
	});

	it("has no link when the viewer cannot open that chat", async () => {
		const [latest, ...older] = MockChatArtifact.versions ?? [];
		renderView({
			...MockChatArtifact,
			versions: [{ ...latest, chat_title: "" }, ...older],
		});
		expect(
			await screen.findByRole("button", { name: "Share artifact" }),
		).toBeInTheDocument();
		expect(
			screen.queryByRole("link", { name: "Open chat" }),
		).not.toBeInTheDocument();
	});
});
