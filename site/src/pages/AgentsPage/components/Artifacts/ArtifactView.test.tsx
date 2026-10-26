import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import {
	MockChatArtifact,
	MockChatArtifactVersion,
} from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import themes from "#/theme";
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

	it.each(["dark", "light"] as const)(
		"loads an HTML version in the %s dashboard theme",
		async (mode) => {
			// coderd stores HTML as text/plain, so the file name picks the frame.
			const mockHtmlVersion = {
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
								<ThemeOverride theme={themes[mode]}>
									<ArtifactView
										artifact={{
											...MockChatArtifact,
											versions: [mockHtmlVersion],
										}}
										selectedVersion={1}
										onSelectVersion={() => undefined}
										currentUserId={MockUserOwner.id}
									/>
								</ThemeOverride>
							),
						},
					],
					{ initialEntries: ["/"] },
				),
			);

			const frame = await screen.findByTitle("report.html");
			expect(frame.getAttribute("src")).toBe(
				`/api/experimental/chats/artifacts/${MockChatArtifact.id}/versions/1/frame?theme=${mode}`,
			);
		},
	);
});
