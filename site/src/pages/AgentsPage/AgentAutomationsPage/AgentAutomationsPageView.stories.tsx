import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { AgentAutomationsPageView } from "./AgentAutomationsPageView";
import { placeholderAutomations } from "./placeholderAutomations";

const meta = {
	title: "pages/AgentsPage/AgentAutomationsPageView",
	component: AgentAutomationsPageView,
	args: {
		automations: placeholderAutomations,
		onEnabledChange: fn(),
		onEditAutomation: fn(),
		onRunAutomation: fn(),
		onViewChats: fn(),
		onDeleteAutomation: fn(),
	},
	decorators: [
		(Story) => (
			<div className="mx-auto w-full max-w-3xl p-4 pt-8">
				<Story />
			</div>
		),
	],
} satisfies Meta<typeof AgentAutomationsPageView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const ActionsMenuOpen: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "More actions for Daily PR review",
			}),
		);
	},
};

export const Filtered: Story = {
	play: async ({ canvasElement }) => {
		const filter = within(canvasElement).getByRole("combobox", {
			name: "Search and filter automations…",
		});
		await userEvent.type(filter, "release");
		await userEvent.keyboard("{Enter}");
	},
};

export const FiltersApplied: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			canvas.getByRole("combobox", {
				name: "Search and filter automations…",
			}),
		);
		await userEvent.click(
			await body.findByRole("option", { name: "Disabled" }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", {
				name: "Search and filter automations…",
			}),
		);
		await userEvent.click(await body.findByRole("option", { name: "Webhook" }));
	},
};

export const Empty: Story = {
	args: { automations: [] },
};

export const Mobile: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
};

export const EmptyMobile: Story = {
	...Empty,
	globals: { viewport: { value: "iphone12", isRotated: false } },
};
