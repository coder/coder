import { renderHook, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { QueryClientProvider } from "react-query";
import { describe, expect, it } from "vitest";
import { chatFileText, chatFileTextKey } from "#/api/queries/chats";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { AgentAttachmentList } from "./AgentAttachmentList";
import { FileProbeProvider, useFileProbes } from "./FileProbeContext";
import { getAgentAttachments } from "./messageHelpers";
import { MockMixedAttachmentMessages } from "./storyFixtures";

const mockFindingsAttachments = getAgentAttachments(
	MockMixedAttachmentMessages,
).filter((attachment) => attachment.file.file_id === "findings-file");

describe("AgentAttachmentList", () => {
	it("marks a text file expired for the timeline once its view finds it deleted", async () => {
		const user = userEvent.setup();
		server.use(
			http.get(
				"/api/v2/chats/files/:fileId",
				() => new HttpResponse(null, { status: 404 }),
			),
		);
		const queryClient = createTestQueryClient();
		await expect(
			queryClient.fetchQuery(chatFileText("findings-file")),
		).rejects.toThrow();
		const { result } = renderHook(() => useFileProbes(), {
			wrapper: ({ children }) => (
				<QueryClientProvider client={queryClient}>
					<TooltipProvider>
						<FileProbeProvider evictedFileIds={new Set()}>
							<AgentAttachmentList attachments={mockFindingsAttachments} />
							{children}
						</FileProbeProvider>
					</TooltipProvider>
				</QueryClientProvider>
			),
		});

		await user.click(screen.getByRole("button", { name: /^findings\.md / }));

		await waitFor(() =>
			expect(result.current.hasExpired("findings-file")).toBe(true),
		);
		expect(
			screen.getByRole("img", { name: "Attachment expired: findings.md" }),
		).toHaveFocus();
	});

	it.each([200, 500])(
		"keeps the latest view's preview when an earlier view settles with %s",
		async (firstStatus) => {
			const user = userEvent.setup();
			const firstText = Promise.withResolvers<string>();
			server.use(
				http.get("/api/v2/chats/files/:fileId", async ({ params }) =>
					HttpResponse.text(
						params.fileId === "first-file"
							? await firstText.promise
							: "[Second](https://docs.example.com)",
						{ status: params.fileId === "first-file" ? firstStatus : 200 },
					),
				),
			);
			const queryClient = createTestQueryClient();
			const [findings] = mockFindingsAttachments;
			const attachments = ["first", "second"].map((name) => ({
				...findings,
				key: name,
				file: { ...findings.file, file_id: `${name}-file`, name: `${name}.md` },
			}));
			renderComponent(
				<QueryClientProvider client={queryClient}>
					<AgentAttachmentList attachments={attachments} />
				</QueryClientProvider>,
			);
			await user.click(
				screen.getByRole("button", { name: "first.md View file" }),
			);
			const secondCard = screen.getByRole("button", {
				name: "second.md View file",
			});
			await user.click(secondCard);
			const secondLink = within(
				await screen.findByRole("dialog", { name: "second.md" }),
			).getByRole("link", { name: "Second" });
			secondLink.focus();

			firstText.resolve("# First");
			await waitFor(() =>
				expect(
					queryClient.getQueryState(chatFileTextKey("first-file")),
				).toMatchObject({
					status: firstStatus === 200 ? "success" : "error",
					data: firstStatus === 200 ? "# First" : undefined,
				}),
			);
			await screen.findByRole("button", {
				name:
					firstStatus === 200
						? "first.md View file"
						: "first.md Request failed with status code 500",
				hidden: true,
			});
			expect(secondLink).toHaveFocus();
			await user.keyboard("{Escape}");
			await waitFor(() => expect(secondCard).toHaveFocus());
		},
	);

	it("opens a text file's preview only on the card that loads it", async () => {
		const user = userEvent.setup();
		let requestCount = 0;
		server.use(
			http.get("/api/v2/chats/files/:fileId", () => {
				requestCount += 1;
				return requestCount === 1
					? new HttpResponse(null, { status: 500 })
					: HttpResponse.text("# Findings");
			}),
		);
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<AgentAttachmentList attachments={mockFindingsAttachments} />
				<AgentAttachmentList attachments={mockFindingsAttachments} />
			</QueryClientProvider>,
		);
		const [failedCard, loadedCard] = screen.getAllByRole("button", {
			name: "findings.md View file",
		});
		await user.click(failedCard);
		await screen.findAllByRole("button", {
			name: "findings.md Request failed with status code 500",
		});

		await user.click(loadedCard);
		await screen.findByRole("dialog", { name: "findings.md" });
	});
});
