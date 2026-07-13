import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { Line } from "#/components/Logs/LogLine";
import { AGENT_LOG_LINE_HEIGHT } from "./AgentLogLine";
import { AgentLogs } from "./AgentLogs";
import { MockLogs, MockSources } from "./mocks";

const meta: Meta<typeof AgentLogs> = {
	title: "modules/resources/AgentLogs",
	component: AgentLogs,
	args: {
		sources: MockSources,
		logs: MockLogs,
		height: MockLogs.length * AGENT_LOG_LINE_HEIGHT,
		overflowed: false,
	},
	parameters: {
		layout: "fullscreen",
	},
};

export default meta;
type Story = StoryObj<typeof AgentLogs>;

export const Default: Story = {};

export const Overflowed: Story = {
	args: {
		className: "max-h-[420px]",
		overflowed: true,
	},
};

const sourceId = MockSources[0].id;

// Exercises ANSI parsing, carriage-return progress-bar redraws, untrusted HTML,
// and a ReDoS payload (Cure53 CDM-02-004) that hung the previous ansi-to-html
// converter. fancy-ansi parses in linear time, so this story renders instantly.
const AnsiLogs: readonly Line[] = [
	{
		id: 1,
		level: "info",
		output:
			"\u001b[31mred\u001b[0m \u001b[32mgreen\u001b[0m \u001b[1mbold\u001b[0m",
		time: "2024-03-14T11:31:04.090715Z",
		sourceId,
	},
	{
		id: 2,
		level: "info",
		output: "downloading... 50%\rdownloading... 100%",
		time: "2024-03-14T11:31:04.090715Z",
		sourceId,
	},
	{
		id: 3,
		level: "info",
		output: 'untrusted <span data-testid="xss">markup</span>',
		time: "2024-03-14T11:31:04.090715Z",
		sourceId,
	},
	{
		id: 4,
		level: "info",
		output: `\u001b[${"1".repeat(50)}`,
		time: "2024-03-14T11:31:04.090715Z",
		sourceId,
	},
];

export const AnsiFormatting: Story = {
	args: {
		logs: AnsiLogs,
		height: AnsiLogs.length * AGENT_LOG_LINE_HEIGHT,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Only the final carriage-return segment is shown.
		await canvas.findByText("downloading... 100%");
		expect(canvas.queryByText(/50%/)).not.toBeInTheDocument();
		// Untrusted markup is escaped, not rendered as an element.
		await canvas.findByText(/untrusted <span data-testid="xss">markup<\/span>/);
		expect(canvas.queryByTestId("xss")).not.toBeInTheDocument();
	},
};
