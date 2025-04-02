import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { MockWorkspaceBuildLogs } from "testHelpers/entities";
import type { Line } from "./LogLine";
import { Logs } from "./Logs";

const meta: Meta<typeof Logs> = {
	title: "components/Logs",
	parameters: { chromatic },
	component: Logs,
	args: {
		lines: MockWorkspaceBuildLogs.map<Line>((log) => ({
			id: String(log.id),
			level: log.log_level,
			time: log.created_at,
			output: log.output,
			sourceId: log.log_source,
		})),
	},
};

export default meta;

type Story = StoryObj<typeof Logs>;

const Default: Story = {};

export { Default as Logs };
