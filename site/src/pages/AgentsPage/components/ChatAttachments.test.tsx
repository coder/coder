import {
	act,
	fireEvent,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { QueryClientProvider } from "react-query";
import { chatFileTextKey } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { ChatAttachments } from "./ChatAttachments";
import { getMessageAttachments } from "./ChatConversation/messageHelpers";

const mockAttachmentMessage: TypesGen.ChatMessage = {
	...MockChatMessage,
	role: "assistant",
	content: [
		{
			type: "file",
			media_type: "text/markdown",
			name: "PLAN.md",
			file_id: "plan-file",
		},
		{
			type: "file",
			media_type: "image/png",
			name: "dashboard.png",
			file_id: "dashboard-file",
		},
		{
			type: "file",
			media_type: "text/plain",
			// "Pasted text", base64-encoded.
			data: "UGFzdGVkIHRleHQ=",
		},
	],
};

const renderAttachments = (queryClient = createTestQueryClient()) =>
	renderComponent(
		<QueryClientProvider client={queryClient}>
			<ChatAttachments
				attachments={getMessageAttachments([mockAttachmentMessage])}
				evictedFileIds={new Set()}
				isLoadingEarlierAttachments={false}
				earlierAttachmentsError={null}
				isRetrying={false}
				onRetry={vi.fn()}
			/>
		</QueryClientProvider>,
	);

const recordFileRequests = (respond: () => Response | Promise<Response>) => {
	const requestedFileIds: string[] = [];
	server.use(
		http.get("/api/v2/chats/files/:fileId", ({ params }) => {
			requestedFileIds.push(String(params.fileId));
			return respond();
		}),
	);
	return requestedFileIds;
};

describe("ChatAttachments", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("previews a text file and views it again without requesting it", async () => {
		const user = userEvent.setup();
		const requestedFileIds = recordFileRequests(() =>
			HttpResponse.text("# Plan"),
		);
		renderAttachments();
		const viewButton = screen.getByRole("button", {
			name: "PLAN.md View file",
		});

		await user.click(viewButton);
		await screen.findByRole("dialog", { name: "PLAN.md" });
		await user.keyboard("{Escape}");
		await waitFor(() => expect(viewButton).toHaveFocus());
		await user.click(viewButton);

		await screen.findByRole("dialog", { name: "PLAN.md" });
		expect(requestedFileIds).toEqual(["plan-file"]);
	});

	it("drops a viewed text file's text from the cache once nothing shows it", async () => {
		const user = userEvent.setup();
		const queryClient = createTestQueryClient();
		recordFileRequests(() => HttpResponse.text("# Plan"));
		const { unmount } = renderAttachments(queryClient);
		await user.click(screen.getByRole("button", { name: "PLAN.md View file" }));
		await waitFor(() =>
			expect(queryClient.getQueryData(chatFileTextKey("plan-file"))).toBe(
				"# Plan",
			),
		);

		unmount();

		await waitFor(() =>
			expect(
				queryClient.getQueryData(chatFileTextKey("plan-file")),
			).toBeUndefined(),
		);
	});

	it("previews pasted text without requesting a file", async () => {
		const user = userEvent.setup();
		const requestedFileIds = recordFileRequests(() => HttpResponse.text(""));
		renderAttachments();

		await user.click(
			screen.getByRole("button", { name: "Pasted text View file" }),
		);

		await screen.findByRole("dialog", { name: "Pasted text" });
		expect(requestedFileIds).toEqual([]);
	});

	it("returns focus to an image card once its preview closes", async () => {
		const user = userEvent.setup();
		renderAttachments();
		const viewButton = screen.getByRole("button", {
			name: "dashboard.png View file",
		});

		await user.click(viewButton);
		await screen.findByRole("dialog", { name: "Image preview" });
		await user.keyboard("{Escape}");

		await waitFor(() => expect(viewButton).toHaveFocus());
	});

	it("shares a viewable file from its download link in the installed iOS app", async () => {
		const user = userEvent.setup();
		const share = vi.fn<Navigator["share"]>().mockResolvedValue(undefined);
		vi.stubGlobal("navigator", {
			userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)",
			standalone: true,
			share,
			canShare: () => true,
		});
		const requestedFileIds = recordFileRequests(() =>
			HttpResponse.text("# Plan"),
		);
		renderAttachments();

		await user.click(screen.getByRole("link", { name: "Download PLAN.md" }));

		await waitFor(() => expect(share).toHaveBeenCalledTimes(1));
		expect(requestedFileIds).toEqual(["plan-file"]);
		expect(share.mock.calls[0][0]?.files?.[0].name).toBe("PLAN.md");
	});

	it("focuses the download card that replaces an image whose thumbnail fails", () => {
		renderAttachments();
		const viewButton = screen.getByRole("button", {
			name: "dashboard.png View file",
		});
		viewButton.focus();

		fireEvent.error(within(viewButton).getByRole("img"));

		expect(
			screen.getByRole("link", { name: "Download dashboard.png" }),
		).toHaveFocus();
	});

	it("focuses the download card when an open image's thumbnail fails", async () => {
		const user = userEvent.setup();
		renderAttachments();
		const viewButton = screen.getByRole("button", {
			name: "dashboard.png View file",
		});
		const thumbnail = within(viewButton).getByRole("img");
		await user.click(viewButton);
		await screen.findByRole("dialog", { name: "Image preview" });

		fireEvent.error(thumbnail);

		await waitFor(() =>
			expect(
				screen.getByRole("link", { name: "Download dashboard.png" }),
			).toHaveFocus(),
		);
	});

	it("opens a text file's preview before its text loads", async () => {
		const user = userEvent.setup();
		recordFileRequests(() => new Promise<never>(() => {}));
		renderAttachments();

		await user.click(screen.getByRole("button", { name: "PLAN.md View file" }));

		expect(
			await screen.findByRole("dialog", { name: "PLAN.md" }),
		).toHaveFocus();
	});

	it("keeps focus on a text file's card once its view finds it deleted", async () => {
		const user = userEvent.setup();
		recordFileRequests(() => new HttpResponse(null, { status: 404 }));
		renderAttachments();

		await user.click(screen.getByRole("button", { name: "PLAN.md View file" }));

		expect(
			await screen.findByRole("button", {
				name: "PLAN.md Attachment expired",
			}),
		).toHaveFocus();
	});

	it("does not open a failed text file's preview when its text loads later", async () => {
		const user = userEvent.setup();
		const queryClient = createTestQueryClient();
		recordFileRequests(() => new HttpResponse(null, { status: 500 }));
		renderAttachments(queryClient);
		await user.click(screen.getByRole("button", { name: "PLAN.md View file" }));
		await screen.findByRole("button", {
			name: "PLAN.md Couldn't load preview. Select again to retry.",
		});

		// Another view of the file loads its text.
		act(() => {
			queryClient.setQueryData(chatFileTextKey("plan-file"), "# Plan");
		});

		// An open preview would take focus from the card.
		expect(
			await screen.findByRole("button", { name: "PLAN.md View file" }),
		).toHaveFocus();
	});
});
