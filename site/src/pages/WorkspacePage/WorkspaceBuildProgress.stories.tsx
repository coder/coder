import type { Meta, StoryObj } from "@storybook/react";
import dayjs from "dayjs";
import {
	MockProvisionerJob,
	MockStartingWorkspace,
	MockWorkspaceBuild,
} from "testHelpers/entities";
import { WorkspaceBuildProgress } from "./WorkspaceBuildProgress";

const meta: Meta<typeof WorkspaceBuildProgress> = {
	title: "pages/WorkspacePage/WorkspaceBuildProgress",
	component: WorkspaceBuildProgress,
	args: {
		transitionStats: {
			P50: 10000,
			P95: 10010,
		},
		workspace: {
			...MockStartingWorkspace,
			latest_build: {
				...MockWorkspaceBuild,
				status: "starting",
				job: {
					...MockProvisionerJob,
					started_at: dayjs().add(-5, "second").format(),
					status: "running",
				},
			},
		},
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceBuildProgress>;

export const Starting: Story = {};

// When the transition stats are returning null, the progress bar should not be
// displayed
export const StartingUnknown: Story = {
	args: {
		transitionStats: {
			P50: null,
			P95: null,
		},
	},
};

export const StartingPassedEstimate: Story = {
	args: {
		transitionStats: { P50: 1000, P95: 1000 },
	},
};

export const StartingHighVariaton: Story = {
	args: {
		transitionStats: { P50: 10000, P95: 20000 },
	},
};

export const StartingZeroEstimate: Story = {
	args: {
		transitionStats: { P50: 0, P95: 0 },
	},
};
