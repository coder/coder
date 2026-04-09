import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Table, TableBody } from "#/components/Table/Table";
import { MockInterception } from "#/testHelpers/entities";
import { RequestLogsRow } from "./RequestLogsRow";

const meta: Meta<typeof RequestLogsRow> = {
	title: "pages/AIBridgePage/RequestLogsRow",
	component: RequestLogsRow,
	decorators: [
		(Story) => (
			<Table>
				<TableBody>
					<Story />
				</TableBody>
			</Table>
		),
	],
};

export default meta;
type Story = StoryObj<typeof RequestLogsRow>;

export const Close: Story = {
	args: {
		interception: MockInterception,
	},
};

export const OpenWithToolCallAndMetadata: Story = {
	args: {
		interception: {
			...MockInterception,
			token_usages: [
				{
					...MockInterception.token_usages[0],
					input_tokens: 1024,
					output_tokens: 256,
					metadata: {
						reasoning_tokens: 128,
						cached_tokens: 512,
					},
				},
			],
			tool_usages: [
				{
					id: "a1b2c3d4-0000-0000-0000-000000000001",
					interception_id: MockInterception.id,
					provider_response_id: "res_tool_001",
					server_url: "https://mcp.example.com/tools",
					tool: "read_file",
					input: JSON.stringify({ path: "/home/coder/project/main.go" }),
					injected: false,
					invocation_error: "",
					metadata: {},
					created_at: MockInterception.started_at,
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const row = canvas.getAllByRole("row")[0];
		await userEvent.click(row);
	},
};

export const OpenWithLongToolCall: Story = {
	args: {
		interception: {
			...MockInterception,
			tool_usages: [
				{
					id: "a1b2c3d4-0000-0000-0000-000000000002",
					interception_id: MockInterception.id,
					provider_response_id: "res_tool_002",
					server_url: "https://mcp.example.com/tools",
					tool: "Write",
					input: JSON.stringify({
						command: {
							file_path:
								"/home/coder/coder/site/src/pages/AIBridgePage/SessionThreadsPage/SessionSummaryTable.tsx",
							// extremely long tool call content
							content:
								'import type { MinimalUser } from "#/api/typesGenerated";\nimport { Avatar } from "#/components/Avatar/Avatar";\nimport { Badge } from "#/components/Badge/Badge";\nimport { AIBridgeClientIcon } from "#/pages/AIBridgePage/RequestLogsPage/icons/AIBridgeClientIcon";\nimport { AIBridgeProviderIcon } from "#/pages/AIBridgePage/RequestLogsPage/icons/AIBridgeProviderIcon";\nimport { cn } from "#/utils/cn";\nimport { formatDateTime } from "#/utils/time";\nimport { TokenBadges } from "../TokenBadges";\nimport { getProviderDisplayName, getProviderIconName } from "../utils";\n\nconst Separator = () =\u003e \u003cdiv className="border-0 border-t border-solid my-1" /\u003e;\n\ninterface SessionSummaryTableProps {\n\tsessionId: string;\n\tstartTime: Date;\n\tendTime?: Date;\n\tinitiator: MinimalUser;\n\tclient: string;\n\tproviders: readonly string[];\n\tinputTokens: number;\n\toutputTokens: number;\n\tthreadCount: number;\n\ttoolCallCount: number;\n\ttokenUsageMetadata?: Record\u003cstring, unknown\u003e;\n}\n\nexport const SessionSummaryTable = ({\n\tsessionId,\n\tstartTime,\n\tendTime,\n\tinitiator,\n\tproviders,\n\tclient,\n\tinputTokens,\n\toutputTokens,\n\tthreadCount,\n\ttoolCallCount,\n\ttokenUsageMetadata,\n}: SessionSummaryTableProps) =\u003e {\n\tconst durationInMs =\n\t\tendTime !== undefined\n\t\t\t? new Date(endTime).getTime() - new Date(startTime).getTime()\n\t\t\t: undefined;\n\n\treturn (\n\t\t\u003cdl className="text-sm text-content-secondary m-0 whitespace-nowrap grid grid-cols-[auto_1fr] gap-y-2"\u003e\n\t\t\t\u003cdt className="h-6 inline-flex items-center font-normal"\u003eSession ID\u003c/dt\u003e\n\t\t\t\u003cdd\n\t\t\t\tclassName="ml-0 text-content-primary h-6 flex min-w-0 items-center justify-end text-xs font-mono"\n\t\t\t\ttitle={sessionId}\n\t\t\t\u003e\n\t\t\t\t\u003cspan className="truncate w-full text-right"\u003e{sessionId}\u003c/span\u003e\n\t\t\t\u003c/dd\u003e\n\n\t\t\t\u003cdt className="h-6 inline-flex items-center font-normal"\u003eStart time\u003c/dt\u003e\n\t\t\t\u003cdd\n\t\t\t\tclassName="ml-0 text-content-primary h-6 flex min-w-0 items-center justify-end text-xs font-mono"\n\t\t\t\ttitle={formatDateTime(startTime)}\n\t\t\t\u003e\n\t\t\t\t{formatDateTime(startTime)}\n\t\t\t\u003c/dd\u003e\n\n\t\t\t\u003cdt className="h-6 inline-flex items-center font-normal"\u003eEnd time\u003c/dt\u003e\n\t\t\t\u003cdd className="ml-0 text-content-primary h-6 flex min-w-0 items-center justify-end text-xs font-mono"\u003e\n\t\t\t\t{endTime ? formatDateTime(endTime) : "—"}\n\t\t\t\u003c/dd\u003e\n\n\t\t\t\u003cdt className="h-6 inline-flex items-center font-normal"\u003eDuration\u003c/dt\u003e\n\t\t\t\u003cdd\n\t\t\t\tclassName="ml-0 text-content-primary h-6 flex min-w-0 items-center justify-end text-xs font-mono"\n\t\t\t\ttitle={durationInMs !== undefined ? `${durationInMs} ms` : undefined}\n\t\t\t\u003e\n\t\t\t\t{durationInMs !== undefined\n\t\t\t\t\t? `${Math.round(durationInMs / 1000)} s`\n\t\t\t\t\t: "—"}\n\t\t\t\u003c/dd\u003e\n\n\t\t\t\u003cdt className="h-6 inline-flex items-center font-normal"\u003eInitiator\u003c/dt\u003e\n\t\t\t\u003cdd className="ml-0 text-content-primary h-6 flex min-w-0 items-center justify-end"\u003e\n\t\t\t\t\u003cdiv className="flex w-full min-w-0 items-center justify-end gap-2"\u003e\n\t\t\t\t\t\u003cAvatar\n\t\t\t\t\t\tsize="sm"\n\t\t\t\t\t\tsrc={initiator.avatar_url}\n\t\t\t\t\t\tfallback={initiator.name}\n\t\t\t\t\t/\u003e\n\t\t\t\t\t\u003cspan className="truncate min-w-0 text-right" title={initiator.name}\u003e\n\t\t\t\t\t\t{initiator.name}\n\t\t\t\t\t\u003c/span\u003e\n\t\t\t\t\u003c/div\u003e\n\t\t\t\u003c/dd\u003e\n\n\t\t\t\u003cdt className="h-6 inline-flex items-center font-normal"\u003eClient\u003c/dt\u003e\n\t\t\t\u003cdd className="ml-0 text-content-primary h-6 flex min-w-0 items-center justify-end"\u003e\n\t\t\t\t\u003cBadge className="gap-1.5 max-w-full min-w-0 overflow-hidden"\u003e\n\t\t\t\t\t\u003cdiv className="flex-shrink-0 flex items-center"\u003e\n\t\t\t\t\t\t\u003cAIBridgeClientIcon client={client} className="size-icon-xs" /\u003e\n\t\t\t\t\t\u003c/div\u003e\n\t\t\t\t\t\u003cspan className="truncate min-w-0 flex-1" title={client ?? "Unknown"}\u003e\n\t\t\t\t\t\t{client ?? "Unknown"}\n\t\t\t\t\t\u003c/span\u003e\n\t\t\t\t\u003c/Badge\u003e\n\t\t\t\u003c/dd\u003e\n\n\t\t\t\u003cdt className="h-6 inline-flex items-center font-normal self-start"\u003e\n\t\t\t\tProvider\n\t\t\t\u003c/dt\u003e\n\t\t\t\u003cdd className="ml-0 text-content-primary h-6 flex min-w-0 items-center justify-end"\u003e\n\t\t\t\t{providers.map((p) =\u003e (\n\t\t\t\t\t\u003cBadge key={p} className="gap-1.5 max-w-full min-w-0 overflow-hidden"\u003e\n\t\t\t\t\t\t\u003cAIBridgeProviderIcon\n\t\t\t\t\t\t\tprovider={getProviderIconName(p)}\n\t\t\t\t\t\t\tclassName="size-icon-xs"\n\t\t\t\t\t\t/\u003e\n\t\t\t\t\t\t\u003cspan\n\t\t\t\t\t\t\tclassName="truncate min-w-0 flex-1"\n\t\t\t\t\t\t\ttitle={getProviderDisplayName(p)}\n\t\t\t\t\t\t\u003e\n\t\t\t\t\t\t\t{getProviderDisplayName(p)}\n\t\t\t\t\t\t\u003c/span\u003e\n\t\t\t\t\t\u003c/Badge\u003e\n\t\t\t\t))}\n\t\t\t\u003c/dd\u003e\n\n\t\t\t\u003cdiv className="col-span-2"\u003e\n\t\t\t\t\u003cSeparator /\u003e\n\t\t\t\u003c/div\u003e\n\n\t\t\t\u003cdt className="h-6 inline-flex items-center font-normal"\u003e\n\t\t\t\tIn / out tokens\n\t\t\t\u003c/dt\u003e\n\t\t\t\u003cdd className="ml-0 text-content-primary h-6 flex min-w-0 items-center justify-end"\u003e\n\t\t\t\t\u003cTokenBadges\n\t\t\t\t\tinputTokens={inputTokens}\n\t\t\t\t\toutputTokens={outputTokens}\n\t\t\t\t\ttokenUsageMetadata={tokenUsageMetadata}\n\t\t\t\t/\u003e\n\t\t\t\u003c/dd\u003e\n\n\t\t\t\u003cdt className="h-6 inline-flex items-center font-normal"\u003eThreads\u003c/dt\u003e\n\t\t\t\u003cdd className="ml-0 text-content-primary h-6 flex min-w-0 items-center justify-end"\u003e\n\t\t\t\t\u003cBadge\u003e{threadCount}\u003c/Badge\u003e\n\t\t\t\u003c/dd\u003e\n\n\t\t\t\u003cdt className="h-6 inline-flex items-center font-normal"\u003eTool calls\u003c/dt\u003e\n\t\t\t\u003cdd className="ml-0 text-content-primary h-6 flex min-w-0 items-center justify-end"\u003e\n\t\t\t\t\u003cBadge\u003e{toolCallCount}\u003c/Badge\u003e\n\t\t\t\u003c/dd\u003e\n\t\t\u003c/dl\u003e\n\t);\n};\n',
						},
						explanation: "Write SessionSummaryTable",
						require_user_approval: false,
					}),
					injected: false,
					invocation_error: "",
					metadata: {},
					created_at: MockInterception.started_at,
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const row = canvas.getAllByRole("row")[0];
		await userEvent.click(row);
	},
};
