import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { InboxButton } from "./InboxButton";

describe("InboxButton", () => {
	it.each([
		{ unreadCount: 0, name: "Notifications" },
		{ unreadCount: 3, name: "Notifications, 3 unread" },
	])(
		"is operable by its name with $unreadCount unread",
		async ({ unreadCount, name }) => {
			const user = userEvent.setup();
			const onClick = vi.fn();
			render(<InboxButton unreadCount={unreadCount} onClick={onClick} />);

			await user.click(screen.getByRole("button", { name }));

			expect(onClick).toHaveBeenCalledTimes(1);
		},
	);
});
