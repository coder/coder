import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { Link } from "#/components/Link/Link";
import { InfoTooltip } from "./InfoTooltip";

const meta = {
	title: "components/InfoTooltip",
	component: InfoTooltip,
	args: {
		type: "info",
		title: "Hello, friend!",
		message: "Today is a lovely day :^)",
	},
} satisfies Meta<typeof InfoTooltip>;

export default meta;
type Story = StoryObj<typeof InfoTooltip>;

export const Info: Story = {
	play: async ({ step }) => {
		await step("hover trigger reveals content", async () => {
			await userEvent.hover(screen.getByRole("button"));
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent(
					meta.args.message,
				),
			);
		});
	},
};

export const Warning = {
	args: {
		type: "warning",
		title: "Something needs attention",
		message: "Unfortunately, there's a radio connected to my brain",
	},
	play: async ({ step }) => {
		await step("hover trigger reveals content", async () => {
			await userEvent.hover(screen.getByRole("button"));
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent(
					Warning.args.message,
				),
			);
		});
	},
} satisfies Story;

export const WithLink = {
	args: {
		title: "What is a role?",
		message: (
			<>
				Coder role-based access control (RBAC) provides fine-grained access
				management. View our docs on how to use the available roles.
				<br />
				<Link size="sm" href="https://coder.com/docs">
					User Roles
				</Link>
			</>
		),
	},
	play: async ({ step }) => {
		await step("hover trigger reveals content", async () => {
			await userEvent.hover(screen.getByRole("button"));
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent("User Roles"),
			);
		});
	},
} satisfies Story;
