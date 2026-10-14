import type { Meta, StoryObj } from "@storybook/react-vite";
import { cn } from "cn";
import { useState } from "react";
import { fn } from "storybook/test";
import { defaultUrlTransform } from "streamdown";
import { preferenceSettingsKey } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	MockChatMessage,
	MockMCPServerConfig,
} from "#/testHelpers/chatEntities";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import { createMockFile } from "#/testHelpers/files";
import {
	withDashboardProvider,
	withProxyProvider,
} from "#/testHelpers/storybook";
import { MessageScroller } from "#/vendor/message-scroller";
import { AgentChatInput, type UploadState } from "./AgentChatInput";
import { ConversationTimeline } from "./ChatConversation/ConversationTimeline";
import { parseMessagesWithMergedTools } from "./ChatConversation/messageParsing";
import {
	buildLiveStatus,
	buildReconnectState,
} from "./ChatConversation/storyFixtures";
import { ChatMessageScroller } from "./ChatMessageScroller";

const scenarios = [
	"Demo data",
	"Worst case",
	"Empty",
	"One",
	"1,000 messages",
	"Reconnect",
	"Upload error",
] as const;
type Scenario = (typeof scenarios)[number];

const draft =
	"Aleksandra Wiśniewska-Kowalczyk: compare 日本語の変更 and مرحبا بالعالم 👩🏽‍💻. Keep the deployment configuration unchanged.";
const longPath =
	"site/src/pages/OrganizationSettingsPage/OrganizationProvisionerSettingsPage/OrganizationProvisionerSettingsPageView.stories.tsx";
const mockDemoMessages: TypesGen.ChatMessage[] = [
	{
		...MockChatMessage,
		content: [{ type: "text", text: "Review the deployment configuration." }],
	},
	{
		...MockChatMessage,
		id: 2,
		role: "assistant",
		content: [{ type: "text", text: "The configuration is ready for review." }],
	},
];
const mockWorstMessages: TypesGen.ChatMessage[] = [
	{ ...MockChatMessage, content: [{ type: "text", text: draft }] },
	{
		...MockChatMessage,
		id: 2,
		role: "assistant",
		content: [
			{
				type: "text",
				text: [
					"## Deployment review",
					"Contact bartholomew.fitzgerald@northwind-industries-holdings.example.com before deploying.",
					`Review \`${longPath}\` and https://northwind-industries-holdings.example.com/platform/engineering/deployment-configuration/release-candidate-validation-results?environment=staging-europe-west.`,
				].join("\n\n"),
			},
		],
	},
	{
		...MockChatMessage,
		id: 3,
		content: [
			{ type: "text", text: "Compare " },
			{
				type: "file-reference",
				file_name: longPath,
				start_line: 999,
				end_line: 1024,
				content: "export const configuration = { retries: 3 };",
			},
			{ type: "text", text: " with the release configuration." },
		],
	},
	{
		...MockChatMessage,
		id: 4,
		role: "assistant",
		content: [
			{
				type: "text",
				text: [
					"| Environment | Artifact | Result |\n| --- | --- | --- |\n| Europe production | northwind-platform-deployment-configuration-release-candidate-validation-results.json | 1 warning |",
					"```sh\ncoder templates push northwind-platform --directory ./infrastructure/environments/production/europe-west/platform-engineering/workspace-templates\n```",
					"JSX to preserve: <DeploymentConfiguration environment={environment} />. Literal entities: &amp; and <b>not HTML</b>.",
				].join("\n\n"),
			},
		],
	},
];
const mockLargeMessages = Array.from(
	{ length: 1000 },
	(_, index): TypesGen.ChatMessage => ({
		...MockChatMessage,
		id: index + 1,
		role: index % 2 === 0 ? "user" : "assistant",
		content: [
			{
				type: "text",
				text:
					index % 2 === 0
						? `Check deployment ${Math.floor(index / 2) + 1} in the staging environment.`
						: `Deployment ${Math.floor(index / 2) + 1}: health checks passed. No configuration changes required.`,
			},
		],
	}),
);
const messagesByScenario = {
	"Demo data": mockDemoMessages,
	"Worst case": mockWorstMessages,
	Empty: [],
	One: [MockChatMessage],
	"1,000 messages": mockLargeMessages,
	Reconnect: mockWorstMessages,
	"Upload error": mockDemoMessages,
} satisfies Record<Scenario, TypesGen.ChatMessage[]>;
const mockFiles = [
	createMockFile(
		"northwind-platform-deployment-configuration-release-candidate-before.txt",
		"text/plain",
	),
	createMockFile(
		"northwind-platform-deployment-configuration-release-candidate-after.txt",
		"text/plain",
	),
];
const mockMCPServer: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	display_name: "Northwind Industries Platform Engineering Knowledge Base",
	auth_type: "none",
	availability: "default_on",
};

type AdversarialChatProps = {
	scenario: Scenario;
	interactive?: boolean;
};

const AdversarialChat: React.FC<AdversarialChatProps> = ({
	scenario,
	interactive = false,
}) => {
	const [selectedOverride, setSelected] = useState<Scenario | undefined>(() => {
		const saved = interactive
			? new URLSearchParams(window.location.search).get("data")
			: null;
		return scenarios.find((value) => value === saved);
	});
	const selected = selectedOverride ?? scenario;
	const [removedFiles, setRemovedFiles] = useState<readonly File[]>([]);
	const worst = selected === "Worst case" || selected === "Reconnect";
	const attachments =
		worst || selected === "Upload error"
			? mockFiles.filter((file) => !removedFiles.includes(file))
			: [];
	const parsedMessages = parseMessagesWithMergedTools(
		messagesByScenario[selected],
	);
	const reconnecting = selected === "Reconnect";
	const initialValues: Record<Scenario, string> = {
		"Demo data": "Check the staging environment.",
		"Worst case": draft,
		Empty: "",
		One: "Check the staging environment.",
		"1,000 messages": "Check the staging environment.",
		Reconnect: draft,
		"Upload error": draft,
	};

	return (
		<div
			className={cn(
				"mx-auto flex h-dvh w-full max-w-3xl flex-col gap-3",
				interactive && "pb-16",
			)}
		>
			<MessageScroller.Provider autoScroll defaultScrollPosition="start">
				<ChatMessageScroller
					hasMoreMessages={false}
					isFetchingMoreMessages={false}
					isHydratingMessages={false}
					hasFetchMoreError={false}
					hasTranscriptRows={parsedMessages.length > 0}
					onFetchMoreMessages={async () => {}}
				>
					<ConversationTimeline
						organizationId="organization-id"
						parsedMessages={parsedMessages}
						urlTransform={defaultUrlTransform}
						mcpServers={[]}
						streamTools={[]}
						liveStatus={buildLiveStatus(
							reconnecting
								? {
										reconnectState: buildReconnectState({
											attempt: 2,
											delayMs: 2000,
										}),
									}
								: {},
						)}
						subagentStatusOverrides={new Map()}
						subagentTitles={new Map()}
						subagentVariants={new Map()}
						isChatCompleted={!reconnecting}
						showDesktopPreviews={false}
						hasActiveStream={reconnecting}
						isAwaitingFirstStreamChunk={false}
						chatStatus={reconnecting ? "running" : null}
						hasMoreMessages={false}
					/>
				</ChatMessageScroller>
			</MessageScroller.Provider>
			<AgentChatInput
				remountKey={scenarios.indexOf(selected)}
				initialValue={initialValues[selected]}
				onSend={fn()}
				onContentChange={fn()}
				onModelChange={fn()}
				isDisabled={false}
				isLoading={false}
				selectedModel="model-config-1"
				modelOptions={[
					{
						id: "model-config-1",
						provider: "openai",
						model: "gpt-4o",
						displayName: worst
							? "Northwind Platform Engineering Production Model"
							: "GPT-4o",
					},
				]}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
				planModeEnabled={worst}
				onPlanModeToggle={fn()}
				isModelCatalogLoading={false}
				isStreaming={reconnecting}
				onInterrupt={fn()}
				attachments={attachments}
				onRemoveAttachment={(attachment) => {
					const file =
						typeof attachment === "number"
							? attachments[attachment]
							: attachment;
					if (file) setRemovedFiles((previous) => [...previous, file]);
				}}
				uploadStates={
					new Map<File, UploadState>(
						mockFiles.map((file, index) => [
							file,
							selected === "Upload error" && index === 0
								? {
										status: "error",
										error:
											"Upload failed: connection interrupted. Remove the attachment and try again.",
									}
								: { status: "uploaded", fileId: `fixture-${index}` },
						]),
					)
				}
				textContents={
					new Map(
						mockFiles.map((file, index) => [
							file,
							`# Deployment configuration ${index + 1}\n${longPath}\n${draft}`,
						]),
					)
				}
				mcpServers={worst ? [mockMCPServer] : []}
				selectedMCPServerIds={worst ? [mockMCPServer.id] : []}
				onMCPSelectionChange={fn()}
			/>
			{interactive && (
				<div
					role="group"
					aria-label="Test dataset"
					className="fixed bottom-2 left-1/2 z-50 flex w-max max-w-[calc(100%-1rem)] -translate-x-1/2 gap-1 overflow-x-auto rounded-lg border border-solid border-border bg-surface-secondary p-1"
				>
					{scenarios.map((value) => (
						<Button
							key={value}
							size="sm"
							variant={selected === value ? "default" : "subtle"}
							aria-pressed={selected === value}
							onClick={() => {
								const url = new URL(window.location.href);
								url.searchParams.set("data", value);
								window.history.replaceState(null, "", url);
								setRemovedFiles([]);
								setSelected(value);
							}}
						>
							{value}
						</Button>
					))}
				</div>
			)}
		</div>
	);
};

const meta: Meta<typeof AdversarialChat> = {
	title: "pages/AgentsPage/AdversarialChat",
	component: AdversarialChat,
	decorators: [withDashboardProvider, withProxyProvider()],
	parameters: {
		layout: "fullscreen",
		queries: [{ key: preferenceSettingsKey, data: MockUserPreferenceSettings }],
	},
	args: { scenario: "Demo data" },
};
export default meta;
type Story = StoryObj<typeof AdversarialChat>;

export const Demo: Story = {};
export const WorstCase: Story = {
	args: { scenario: "Worst case" },
	parameters: { pixel: { matrix: { viewports: ["phone", "laptop"] } } },
};
export const Empty: Story = { args: { scenario: "Empty" } };
export const One: Story = { args: { scenario: "One" } };
export const Large: Story = { args: { scenario: "1,000 messages" } };
export const Reconnect: Story = { args: { scenario: "Reconnect" } };
export const UploadError: Story = { args: { scenario: "Upload error" } };
export const Explorer: Story = { args: { interactive: true } };
