import type { Meta, StoryObj } from "@storybook/react";
import { Button } from "../Button/Button";
import { CollapsibleSummary } from "./CollapsibleSummary";

const meta: Meta<typeof CollapsibleSummary> = {
	title: "components/CollapsibleSummary",
	component: CollapsibleSummary,
	args: {
		label: "Advanced options",
		children: (
			<>
				<div className="p-2 border border-border rounded-md border-solid">
					Option 1
				</div>
				<div className="p-2 border border-border rounded-md border-solid">
					Option 2
				</div>
				<div className="p-2 border border-border rounded-md border-solid">
					Option 3
				</div>
			</>
		),
	},
};

export default meta;
type Story = StoryObj<typeof CollapsibleSummary>;

export const Default: Story = {};

export const DefaultOpen: Story = {
	args: {
		defaultOpen: true,
	},
};

export const MediumSize: Story = {
	args: {
		size: "md",
	},
};

export const SmallSize: Story = {
	args: {
		size: "sm",
	},
};

export const CustomClassName: Story = {
	args: {
		className: "text-blue-500 font-bold",
	},
};

export const ManyChildren: Story = {
	args: {
		defaultOpen: true,
		children: (
			<>
				{Array.from({ length: 10 }).map((_, i) => (
					<div
						key={`option-${i + 1}`}
						className="p-2 border border-border rounded-md border-solid"
					>
						Option {i + 1}
					</div>
				))}
			</>
		),
	},
};

export const NestedCollapsible: Story = {
	args: {
		defaultOpen: true,
		children: (
			<>
				<div className="p-2 border border-border rounded-md border-solid">
					Option 1
				</div>
				<CollapsibleSummary label="Nested options" size="sm">
					<div className="p-2 border border-border rounded-md border-solid">
						Nested Option 1
					</div>
					<div className="p-2 border border-border rounded-md border-solid">
						Nested Option 2
					</div>
				</CollapsibleSummary>
				<div className="p-2 border border-border rounded-md border-solid">
					Option 3
				</div>
			</>
		),
	},
};

export const ComplexContent: Story = {
	args: {
		defaultOpen: true,
		children: (
			<div className="p-4 border border-border rounded-md bg-surface-secondary">
				<h3 className="text-lg font-bold mb-2">Complex Content</h3>
				<p className="mb-4">
					This is a more complex content example with various elements.
				</p>
				<div className="flex gap-2">
					<Button>Action 1</Button>
					<Button>Action 2</Button>
				</div>
			</div>
		),
	},
};

export const LongLabel: Story = {
	args: {
		label:
			"This is a very long label that might wrap or cause layout issues if not handled properly",
	},
};
