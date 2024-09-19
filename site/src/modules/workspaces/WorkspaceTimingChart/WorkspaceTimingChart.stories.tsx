import type { Meta, StoryObj } from "@storybook/react";
import { WorkspaceTimingChart } from "./WorkspaceTimingChart";
import { WorkspaceTimingsResponse } from "./storybookData";

const meta: Meta<typeof WorkspaceTimingChart> = {
	title: "modules/workspaces/WorkspaceTimingChart",
	component: WorkspaceTimingChart,
	args: {
		provisionerTimings: WorkspaceTimingsResponse.provisioner_timings,
	},
	decorators: [
		(Story) => {
			return (
				<div
					css={(theme) => ({
						borderRadius: 8,
						border: `1px solid ${theme.palette.divider}`,
						width: 1200,
						height: 420,
						overflow: "auto",
					})}
				>
					<Story />
				</div>
			);
		},
	],
};

export default meta;
type Story = StoryObj<typeof WorkspaceTimingChart>;

export const Default: Story = {};
