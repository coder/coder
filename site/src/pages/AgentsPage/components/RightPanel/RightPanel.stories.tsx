import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { RightPanel } from "./RightPanel";

const meta: Meta<typeof RightPanel> = {
	title: "pages/AgentsPage/RightPanel",
	component: RightPanel,
	args: {
		isOpen: true,
		isExpanded: false,
		onToggleExpanded: fn(),
		onClose: fn(),
		children: (
			<div
				style={{
					display: "flex",
					alignItems: "center",
					justifyContent: "center",
					height: "100%",
					padding: 24,
					color: "var(--content-secondary)",
				}}
			>
				Panel content
			</div>
		),
	},
	decorators: [
		(Story) => (
			<div style={{ display: "flex", height: 400, width: "100%" }}>
				<div
					style={{
						flex: 1,
						display: "flex",
						alignItems: "center",
						justifyContent: "center",
						color: "var(--content-secondary)",
					}}
				>
					Main content area
				</div>
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof RightPanel>;

export const Default: Story = {};

export const Closed: Story = {
	args: { isOpen: false },
};

export const Expanded: Story = {
	args: { isExpanded: true },
};
