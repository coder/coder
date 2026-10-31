import { screen } from "@testing-library/react";
import { MockNotification } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { InboxItem } from "./InboxItem";

const renderWithContent = (content: string) =>
	render(
		<InboxItem
			notification={{ ...MockNotification, actions: [], content }}
			onMarkNotificationAsRead={vi.fn()}
		/>,
	);

describe("InboxItem", () => {
	it("opens external markdown links in a new tab", async () => {
		renderWithContent("Marked [dormant](https://docs.example.com/dormancy).");

		expect(await screen.findByRole("link")).toHaveAccessibleName(
			/^dormant\s*\(opens in new tab\)$/,
		);
	});

	it("keeps relative markdown links in the same tab", async () => {
		renderWithContent("See [your workspaces](/workspaces).");

		expect(await screen.findByRole("link")).toHaveAccessibleName(
			"your workspaces",
		);
	});

	it("keeps absolute links to this deployment in the same tab", async () => {
		renderWithContent(
			`See [the build](${location.origin}/@admin/dev/builds/2).`,
		);

		expect(await screen.findByRole("link")).toHaveAccessibleName("the build");
	});
});
