import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import {
	MockChatArtifact,
	MockChatArtifactVersion,
} from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { ArtifactView } from "./ArtifactView";

describe("ArtifactView", () => {
	it("opens the chat of the latest version, whichever version shows", async () => {
		const user = userEvent.setup();
		const { router } = renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/",
						element: (
							<ArtifactView
								artifact={MockChatArtifact}
								selectedVersion={1}
								onSelectVersion={() => undefined}
								currentUserId={MockUserOwner.id}
								showChatLink
							/>
						),
					},
					{ path: "/agents/:chatId", element: <div /> },
				],
				{ initialEntries: ["/"] },
			),
		);

		await user.click(await screen.findByRole("link", { name: "Open chat" }));

		expect(router.state.location.pathname).toBe("/agents/chat-1");
	});

	it("loads an HTML version in the dashboard theme", async () => {
		const html = {
			...MockChatArtifactVersion,
			name: "report.html",
			media_type: "text/plain",
		};
		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/",
						element: (
							<ArtifactView
								artifact={{ ...MockChatArtifact, versions: [html] }}
								selectedVersion={1}
								onSelectVersion={() => undefined}
								currentUserId={MockUserOwner.id}
							/>
						),
					},
				],
				{ initialEntries: ["/"] },
			),
		);

		const frame = await screen.findByTitle("report.html");
		expect(frame.getAttribute("src")).toBe(
			`/api/experimental/chats/artifacts/${MockChatArtifact.id}/versions/1/frame?theme=dark`,
		);
	});
});
