import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { AdminBadge } from "./AdminBadge";
import {
	CollapsibleSection,
	CollapsibleSectionContent,
	CollapsibleSectionDescription,
	CollapsibleSectionHeader,
	CollapsibleSectionTitle,
} from "./CollapsibleSection";

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
	<p className="text-sm text-content-secondary">Placeholder content</p>
);

export const DefaultOpen: Story = {
	render: () => (
		<CollapsibleSection>
			<CollapsibleSectionHeader>
				<div className="flex items-center gap-2">
					<CollapsibleSectionTitle>Default spend limit</CollapsibleSectionTitle>
					<AdminBadge />
				</div>
				<CollapsibleSectionDescription>
					The deployment-wide spending cap.
				</CollapsibleSectionDescription>
			</CollapsibleSectionHeader>
			<CollapsibleSectionContent>
				<Placeholder />
			</CollapsibleSectionContent>
		</CollapsibleSection>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();

		const header = canvas.getByRole("button", {
			name: /Default spend limit/i,
		});
		expect(header).toHaveAttribute("aria-expanded", "true");

		await userEvent.click(header);
		expect(header).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();

		await userEvent.click(header);
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();
	},
};

export const Collapsed: Story = {
	render: () => (
		<CollapsibleSection defaultOpen={false}>
			<CollapsibleSectionHeader>
				<div className="flex items-center gap-2">
					<CollapsibleSectionTitle>Default spend limit</CollapsibleSectionTitle>
					<AdminBadge />
				</div>
				<CollapsibleSectionDescription>
					The deployment-wide spending cap.
				</CollapsibleSectionDescription>
			</CollapsibleSectionHeader>
			<CollapsibleSectionContent>
				<Placeholder />
			</CollapsibleSectionContent>
		</CollapsibleSection>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();

		const header = canvas.getByRole("button", {
			name: /Default spend limit/i,
		});
		expect(header).toHaveAttribute("aria-expanded", "false");

		await userEvent.click(header);
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();

		await userEvent.click(header);
		expect(header).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();
	},
};

export const NoBadge: Story = {
	render: () => (
		<CollapsibleSection>
			<CollapsibleSectionHeader>
				<CollapsibleSectionTitle>Group limits</CollapsibleSectionTitle>
				<CollapsibleSectionDescription>
					Override defaults for groups.
				</CollapsibleSectionDescription>
			</CollapsibleSectionHeader>
			<CollapsibleSectionContent>
				<Placeholder />
			</CollapsibleSectionContent>
		</CollapsibleSection>
	),
};

export const InlineVariant: Story = {
	render: () => (
		<CollapsibleSection variant="inline" defaultOpen={false}>
			<CollapsibleSectionHeader>
				<CollapsibleSectionTitle as="h3">Cost Tracking</CollapsibleSectionTitle>
				<CollapsibleSectionDescription>
					Set per-token pricing so Coder can track costs and enforce spending
					limits.
				</CollapsibleSectionDescription>
			</CollapsibleSectionHeader>
			<CollapsibleSectionContent>
				<Placeholder />
			</CollapsibleSectionContent>
		</CollapsibleSection>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();

		const header = canvas.getByRole("button", {
			name: /Cost Tracking/i,
		});
		expect(header).toHaveAttribute("aria-expanded", "false");

		await userEvent.click(header);
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();

		await userEvent.click(header);
		expect(header).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();
	},
};

export const KeyboardToggle: Story = {
	render: () => (
		<CollapsibleSection>
			<CollapsibleSectionHeader>
				<CollapsibleSectionTitle>Keyboard section</CollapsibleSectionTitle>
			</CollapsibleSectionHeader>
			<CollapsibleSectionContent>
				<Placeholder />
			</CollapsibleSectionContent>
		</CollapsibleSection>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const header = canvas.getByRole("button", {
			name: /Keyboard section/i,
		});
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();

		header.focus();

		await userEvent.keyboard("{Enter}");
		expect(header).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();

		await userEvent.keyboard("{Enter}");
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();

		await userEvent.keyboard(" ");
		expect(header).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("Placeholder content")).not.toBeInTheDocument();

		await userEvent.keyboard(" ");
		expect(header).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Placeholder content")).toBeInTheDocument();
	},
};
