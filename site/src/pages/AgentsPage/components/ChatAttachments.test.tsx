import {
	fireEvent,
	render,
	renderHook,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { ChatAttachments } from "./ChatAttachments";
import {
	FileProbeProvider,
	useFileProbes,
} from "./ChatConversation/FileProbeContext";
import type { AgentAttachment } from "./ChatConversation/messageHelpers";

const storedChart: AgentAttachment = {
	key: "chart",
	file: {
		type: "file",
		media_type: "image/png",
		name: "chart.png",
		file_id: "chart-file",
	},
	href: "/api/v2/chats/files/chart-file",
};

const failThumbnail = () =>
	fireEvent.error(
		within(
			screen.getByRole("button", { name: "chart.png View file" }),
		).getByRole("img"),
	);

describe("ChatAttachments", () => {
	it.each([
		{
			name: "loads earlier messages when asked",
			isFetchingMoreMessages: false,
			calls: 1,
		},
		{
			name: "ignores clicks while a page loads",
			isFetchingMoreMessages: true,
			calls: 0,
		},
	])("$name", async ({ isFetchingMoreMessages, calls }) => {
		const user = userEvent.setup();
		const onFetchMoreMessages = vi.fn().mockResolvedValue(undefined);
		render(
			<ChatAttachments
				attachments={[]}
				evictedFileIds={new Set()}
				hasMoreMessages
				isFetchingMoreMessages={isFetchingMoreMessages}
				hasFetchMoreError={false}
				onFetchMoreMessages={onFetchMoreMessages}
			/>,
		);

		await user.click(
			screen.getByRole("button", { name: /Load earlier messages/ }),
		);

		expect(onFetchMoreMessages).toHaveBeenCalledTimes(calls);
	});

	it("puts an expired file in the tab order under its name", async () => {
		const user = userEvent.setup();
		renderComponent(
			<ChatAttachments
				attachments={[
					{
						key: "old-report",
						file: {
							type: "file",
							media_type: "application/pdf",
							name: "old-report.pdf",
							file_id: "expired-file",
						},
						href: "/api/v2/chats/files/expired-file",
					},
				]}
				evictedFileIds={new Set(["expired-file"])}
				hasMoreMessages={false}
				isFetchingMoreMessages={false}
				hasFetchMoreError={false}
				onFetchMoreMessages={vi.fn()}
			/>,
		);

		await user.tab();
		await user.tab();

		expect(
			screen.getByRole("img", { name: "Attachment expired: old-report.pdf" }),
		).toHaveFocus();
	});

	it("moves focus to a stored image's expired tile once its probe finds it deleted", async () => {
		server.use(
			http.get(
				"/api/v2/chats/files/:fileId",
				() => new HttpResponse(null, { status: 404 }),
			),
		);
		renderComponent(
			<ChatAttachments
				attachments={[storedChart]}
				evictedFileIds={new Set()}
				hasMoreMessages={false}
				isFetchingMoreMessages={false}
				hasFetchMoreError={false}
				onFetchMoreMessages={vi.fn()}
			/>,
		);
		screen.getByRole("button", { name: "chart.png View file" }).focus();

		failThumbnail();

		await waitFor(() =>
			expect(
				screen.getByRole("img", { name: "Attachment expired: chart.png" }),
			).toHaveFocus(),
		);
	});

	it("shares a deleted file with the chat's other file probes", async () => {
		server.use(
			http.get(
				"/api/v2/chats/files/:fileId",
				() => new HttpResponse(null, { status: 404 }),
			),
		);
		const { result } = renderHook(() => useFileProbes(), {
			wrapper: ({ children }) => (
				<TooltipProvider>
					<FileProbeProvider>
						<ChatAttachments
							attachments={[storedChart]}
							evictedFileIds={new Set()}
							hasMoreMessages={false}
							isFetchingMoreMessages={false}
							hasFetchMoreError={false}
							onFetchMoreMessages={vi.fn()}
						/>
						<FileProbeProvider evictedFileIds={new Set()}>
							{children}
						</FileProbeProvider>
					</FileProbeProvider>
				</TooltipProvider>
			),
		});

		failThumbnail();

		await waitFor(() =>
			expect(result.current.hasExpired("chart-file")).toBe(true),
		);
	});
});
