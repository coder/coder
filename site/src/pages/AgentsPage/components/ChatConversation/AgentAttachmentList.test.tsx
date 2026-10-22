import { renderHook, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { QueryClientProvider } from "react-query";
import { describe, expect, it } from "vitest";
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

const findingsAttachments = getAgentAttachments(
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
		const { result } = renderHook(() => useFileProbes(), {
			wrapper: ({ children }) => (
				<QueryClientProvider client={queryClient}>
					<TooltipProvider>
						<FileProbeProvider evictedFileIds={new Set()}>
							<AgentAttachmentList
								attachments={findingsAttachments}
								isExpired={() => false}
							/>
							{children}
						</FileProbeProvider>
					</TooltipProvider>
				</QueryClientProvider>
			),
		});

		await user.click(
			screen.getByRole("button", { name: "findings.md View file" }),
		);

		await waitFor(() =>
			expect(result.current.hasExpired("findings-file")).toBe(true),
		);
	});

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
		// Two lists of one file share its text, like the Summary panel and a
		// turn's popover.
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<AgentAttachmentList
					attachments={findingsAttachments}
					isExpired={() => false}
				/>
				<AgentAttachmentList
					attachments={findingsAttachments}
					isExpired={() => false}
				/>
			</QueryClientProvider>,
		);
		const [failedCard, loadingCard] = screen.getAllByRole("button", {
			name: "findings.md View file",
		});
		await user.click(failedCard);
		await screen.findAllByRole("button", {
			name: "findings.md Couldn't load preview. Select again to retry.",
		});

		await user.click(loadingCard);
		await screen.findByRole("dialog", { name: "findings.md" });
		await user.keyboard("{Escape}");

		await waitFor(() => expect(loadingCard).toHaveFocus());
	});
});
