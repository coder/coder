import { MockBuildInfo, MockUserOwner } from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { UserDropdown } from "./UserDropdown";

const meta: Meta<typeof UserDropdown> = {
	title: "modules/dashboard/UserDropdown",
	component: UserDropdown,
	args: {
		user: MockUserOwner,
		buildInfo: MockBuildInfo,
		supportLinks: [
			{ icon: "docs", name: "Documentation", target: "" },
			{ icon: "bug", name: "Report a bug", target: "" },
			{ icon: "chat", name: "Join the Coder Discord", target: "" },
			{ icon: "star", name: "Star the Repo", target: "" },
			{ icon: "/icon/aws.svg", name: "Amazon Web Services", target: "" },
		],
	},
	decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof UserDropdown>;

const Example: Story = {
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("click to open", async () => {
			await userEvent.click(canvas.getByRole("button"));
			await waitFor(async () =>
				expect(await screen.findByText(/v2\.\d+\.\d+/i)).toBeInTheDocument(),
			);
		});
	},
};

export { Example as UserDropdown };
