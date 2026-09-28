import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import { Link } from "#/components/Link/Link";
import { TooltipMessage, TooltipTitle } from "#/components/Tooltip/Tooltip";
import { InfoTooltip } from "./InfoTooltip";

const meta = {
	title: "components/InfoTooltip",
	component: InfoTooltip,
	args: {
		type: "info",
		children: (
			<>
				<TooltipTitle>Hello, friend!</TooltipTitle>
				<TooltipMessage>Today is a lovely day :^)</TooltipMessage>
			</>
		),
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
					"Today is a lovely day :^)",
				),
			);
		});
	},
};

export const Warning: Story = {
	args: {
		type: "warning",
		children: (
			<>
				<TooltipTitle>Something needs attention</TooltipTitle>
				<TooltipMessage>
					Unfortunately, there's a radio connected to my brain
				</TooltipMessage>
			</>
		),
	},
	play: async ({ step }) => {
		await step("hover trigger reveals content", async () => {
			await userEvent.hover(screen.getByRole("button"));
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent(
					"Unfortunately, there's a radio connected to my brain",
				),
			);
		});
	},
};

export const WithLink: Story = {
	args: {
		children: (
			<>
				<TooltipTitle>What is a role?</TooltipTitle>
				<TooltipMessage>
					Coder role-based access control (RBAC) provides fine-grained access
					management. View our docs on how to use the available roles.
					<Link size="sm" href="https://coder.com/docs">
						User Roles
					</Link>
				</TooltipMessage>
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
};
