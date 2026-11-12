import type { Meta, StoryObj } from "@storybook/react-vite";
import AutoSizer from "react-virtualized-auto-sizer";
import { screen, userEvent, within } from "storybook/test";
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

export const SourceIconFocused: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getAllByRole("button", { name: "Coder Login" })[0]
			.focus();
		await screen.findByRole("tooltip");
	},
};

export const Overflowed: Story = {
	args: {
		height: 256,
		overflowed: true,
	},
	// AgentRow renders the logs inside AutoSizer, whose own div is 0px wide.
	render: (args) => (
		<AutoSizer disableHeight>
			{({ width }) => <AgentLogs {...args} width={width} />}
		</AutoSizer>
	),
};

export const OverflowedTooltip: Story = {
	...Overflowed,
	play: async ({ canvasElement }) => {
		await userEvent.hover(within(canvasElement).getByText("Logs overflowed"));
		await screen.findByRole("tooltip");
	},
};

const sourceId = MockSources[0].id;

// Demonstrates how ANSI escape sequences and carriage-return progress-bar
// redraws are rendered in the log viewer.
const AnsiLogs: readonly Line[] = [
	{
		id: 1,
		level: "info",
		output:
			"\u001b[31mred\u001b[0m \u001b[32mgreen\u001b[0m \u001b[34mblue\u001b[0m \u001b[1mbold\u001b[0m \u001b[3mitalic\u001b[0m",
		time: "2024-03-14T11:31:04.090715Z",
		sourceId,
	},
	{
		id: 2,
		level: "info",
		output: "\u001b[43m\u001b[30m warning \u001b[0m background colors",
		time: "2024-03-14T11:31:04.090715Z",
		sourceId,
	},
	{
		id: 3,
		level: "info",
		output: "downloading... 50%\rdownloading... 100%",
		time: "2024-03-14T11:31:04.090715Z",
		sourceId,
	},
];

export const AnsiFormatting: Story = {
	args: {
		logs: AnsiLogs,
		height: AnsiLogs.length * AGENT_LOG_LINE_HEIGHT,
	},
};
