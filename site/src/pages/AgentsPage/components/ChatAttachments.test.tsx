import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
});
