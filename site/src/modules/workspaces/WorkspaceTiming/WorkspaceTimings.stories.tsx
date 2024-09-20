import type { Meta, StoryObj } from "@storybook/react";
import { WorkspaceTimings } from "./WorkspaceTimings";
import { WorkspaceTimingsResponse } from "./storybookData";

const meta: Meta<typeof WorkspaceTimings> = {
	title: "modules/workspaces/WorkspaceTimings",
	component: WorkspaceTimings,
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
type Story = StoryObj<typeof WorkspaceTimings>;

export const Default: Story = {};
