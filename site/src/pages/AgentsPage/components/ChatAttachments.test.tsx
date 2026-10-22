import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ChatAttachments } from "./ChatAttachments";

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
});
