import { render } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import { InboxPopover } from "./InboxPopover";

describe("InboxPopover", () => {
	it("adds an accessible name to the notifications trigger button", () => {
		render(
			<InboxPopover
				notifications={undefined}
				unreadCount={0}
				error={undefined}
				isLoadingMoreNotifications={false}
				hasMoreNotifications={false}
				onRetry={vi.fn()}
				onMarkAllAsRead={vi.fn()}
				onMarkNotificationAsRead={vi.fn()}
				onLoadMoreNotifications={vi.fn()}
			/>,
		);

		expect(
			screen.getByRole("button", { name: /notifications/i }),
		).toBeInTheDocument();
	});
});
