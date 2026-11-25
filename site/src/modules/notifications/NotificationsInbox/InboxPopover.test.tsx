import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockNotifications } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { InboxPopover } from "./InboxPopover";

describe("InboxPopover", () => {
	it("is operable through the dialog named after its title", async () => {
		const user = userEvent.setup();
		const onMarkAllAsRead = vi.fn();
		render(
			<InboxPopover
				notifications={MockNotifications}
				unreadCount={2}
				error={undefined}
				isLoadingMoreNotifications={false}
				hasMoreNotifications={false}
				onRetry={vi.fn()}
				onMarkAllAsRead={onMarkAllAsRead}
				onMarkNotificationAsRead={vi.fn()}
				onLoadMoreNotifications={vi.fn()}
			/>,
		);

		await user.click(screen.getByRole("button", { name: /notifications/i }));
		const dialog = await screen.findByRole("dialog", { name: "Inbox" });
		await user.click(
			within(dialog).getByRole("button", { name: "Mark all as read" }),
		);

		expect(onMarkAllAsRead).toHaveBeenCalledTimes(1);
	});
});
