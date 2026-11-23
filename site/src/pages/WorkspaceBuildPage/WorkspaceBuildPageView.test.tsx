import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockWorkspaceBuild } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { WorkspaceBuildPageView } from "./WorkspaceBuildPageView";

const mockBuilds = [1, 2, 3].map((buildNumber) => ({
	...MockWorkspaceBuild,
	id: `build-${buildNumber}`,
	build_number: buildNumber,
}));

describe("WorkspaceBuildPageView builds sidebar", () => {
	it("tabs once per build and marks the active build as the current page", async () => {
		const user = userEvent.setup();
		render(
			<WorkspaceBuildPageView
				build={mockBuilds[1]}
				builds={mockBuilds}
				logs={[]}
				activeBuildNumber={2}
			/>,
		);

		const sidebar = screen.getByRole("navigation");
		const [first, active, last] = within(sidebar).getAllByRole("link");
		expect(within(sidebar).getByRole("link", { current: "page" })).toBe(active);

		act(() => first.focus());
		await user.tab();
		expect(active).toHaveFocus();
		await user.tab();
		expect(last).toHaveFocus();
	});

	it("describes the focused build link with the build reason", async () => {
		render(
			<WorkspaceBuildPageView
				build={mockBuilds[0]}
				builds={[{ ...mockBuilds[0], reason: "cli" }]}
				logs={[]}
				activeBuildNumber={2}
			/>,
		);

		const link = within(screen.getByRole("navigation")).getByRole("link");
		act(() => link.focus());

		await waitFor(() => expect(link).toHaveAccessibleDescription("CLI"));
	});
});
