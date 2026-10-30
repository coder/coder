import { screen, within } from "@testing-library/react";
import { MockNotifications } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { InboxPopover } from "./InboxPopover";

describe("InboxPopover", () => {
	it("exposes notifications as a list so each item reports its position", async () => {
		const notifications = MockNotifications.slice(0, 3);
		render(
			<InboxPopover
				defaultOpen
				notifications={notifications}
				unreadCount={0}
				error={undefined}
				isLoadingMoreNotifications={false}
				hasMoreNotifications
				onRetry={vi.fn()}
				onMarkAllAsRead={vi.fn()}
				onMarkNotificationAsRead={vi.fn()}
				onLoadMoreNotifications={vi.fn()}
			/>,
		);

		const list = await screen.findByRole("list", { name: "Notifications" });
		expect(within(list).getAllByRole("listitem")).toHaveLength(
			notifications.length,
		);
		expect(
			within(list).queryByRole("button", { name: /load more/i }),
		).toBeNull();
	});
});
