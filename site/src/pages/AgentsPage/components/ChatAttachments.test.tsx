import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ChatAttachments } from "./ChatAttachments";

describe("ChatAttachments", () => {
	it("loads earlier messages when asked", async () => {
		const user = userEvent.setup();
		const onFetchMoreMessages = vi.fn().mockResolvedValue(undefined);
		render(
			<ChatAttachments
				attachments={[]}
				evictedFileIds={new Set()}
				hasMoreMessages
				isFetchingMoreMessages={false}
				isHydratingMessages={false}
				hasFetchMoreError={false}
				onFetchMoreMessages={onFetchMoreMessages}
			/>,
		);

		await user.click(
			screen.getByRole("button", { name: "Load earlier messages" }),
		);

		expect(onFetchMoreMessages).toHaveBeenCalledTimes(1);
	});
});
