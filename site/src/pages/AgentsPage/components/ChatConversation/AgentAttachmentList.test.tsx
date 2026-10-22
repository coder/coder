import { renderHook, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { QueryClientProvider } from "react-query";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { AgentAttachmentList } from "./AgentAttachmentList";
import { FileProbeProvider, useFileProbes } from "./FileProbeContext";
import { getAgentAttachments } from "./messageHelpers";
import { MockMixedAttachmentMessages } from "./storyFixtures";

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
		const findings = getAgentAttachments(MockMixedAttachmentMessages).filter(
			(attachment) => attachment.file.file_id === "findings-file",
		);
		const { result } = renderHook(() => useFileProbes(), {
			wrapper: ({ children }) => (
				<QueryClientProvider client={queryClient}>
					<TooltipProvider>
						<FileProbeProvider evictedFileIds={new Set()}>
							<AgentAttachmentList
								attachments={findings}
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
});
