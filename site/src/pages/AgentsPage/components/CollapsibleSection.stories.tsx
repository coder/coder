import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { CollapsibleSection } from "./CollapsibleSection";

const meta: Meta<typeof CollapsibleSection> = {
	title: "pages/AgentsPage/CollapsibleSection",
	component: CollapsibleSection,
	decorators: [
		(Story) => (
			<div style={{ maxWidth: 600 }}>
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof CollapsibleSection>;

const Placeholder = () => (
	<div className="h-20 rounded bg-surface-tertiary p-4">
		Placeholder content
	</div>
);

export const DefaultOpen: Story = {
	args: {
		title: "Default spend limit",
		description: "The deployment-wide spending cap.",
		badge: (
			<span className="rounded bg-surface-tertiary px-2 py-0.5 text-xs font-medium text-content-secondary">
				Admin
			</span>
		),
		children: <Placeholder />,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Content should be visible when open by default.
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();

		const header = canvas.getByRole("button", {
			name: /Default spend limit/i,
		});
		expect(header).toHaveAttribute("aria-expanded", "true");

		// Click header to collapse.
		await userEvent.click(header);
		expect(header).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();

		// Click again to re-expand.
		await userEvent.click(header);
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();
	},
};

export const Collapsed: Story = {
	args: {
		...DefaultOpen.args,
		defaultOpen: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Content should be hidden when collapsed by default.
		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();

		const header = canvas.getByRole("button", {
			name: /Default spend limit/i,
		});
		expect(header).toHaveAttribute("aria-expanded", "false");

		// Click header to expand.
		await userEvent.click(header);
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();

		// Click again to collapse.
		await userEvent.click(header);
		expect(header).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();
	},
};

export const WithAction: Story = {
	args: {
		title: "Per-user spend",
		action: (
			<span className="text-xs text-content-secondary">Last 30 days</span>
		),
		children: <Placeholder />,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Action slot should be visible.
		expect(canvas.getByText("Last 30 days")).toBeInTheDocument();

		const header = canvas.getByRole("button", {
			name: /Per-user spend/i,
		});
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();

		// Clicking the action slot should not toggle the section
		// because the wrapper calls stopPropagation.
		await userEvent.click(canvas.getByText("Last 30 days"));
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();
	},
};

export const NoBadge: Story = {
	args: {
		title: "Group limits",
		description: "Override defaults for groups.",
		children: <Placeholder />,
	},
};

export const KeyboardToggle: Story = {
	args: {
		title: "Keyboard section",
		children: <Placeholder />,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const header = canvas.getByRole("button", {
			name: /Keyboard section/i,
		});
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();

		// Focus the header so keyboard events target it.
		header.focus();

		// Press Enter to collapse.
		await userEvent.keyboard("{Enter}");
		expect(header).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();

		// Press Enter to expand.
		await userEvent.keyboard("{Enter}");
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();

		// Press Space to collapse.
		await userEvent.keyboard(" ");
		expect(header).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();

		// Press Space to expand.
		await userEvent.keyboard(" ");
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();
	},
};
