import type { Meta, StoryObj } from "@storybook/react-vite";
import { Link } from "react-router";
import { fn, screen, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModel,
	MockCompactionChatModel,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import {
	compactionOverrideWindowPercent,
	type OrganizationCompactionTrigger,
} from "../utils/compactionTriggers";
import { UserCompactionThresholdSettings } from "./UserCompactionThresholdSettings";

const modelsOrganization = {
	...MockDefaultOrganization,
	id: MockChatModel.organization_id,
};

const organizationWithEmptyDisplayName = {
	...MockDefaultOrganization,
	id: MockChatModel.organization_id,
	display_name: "",
};

const organizationTriggerFor = (
	model: TypesGen.ChatModel,
): OrganizationCompactionTrigger => ({
	model,
	trigger: {
		thresholdPercent: compactionOverrideWindowPercent,
		contextLimit: model.context_limit,
	},
	pointTokens: (model.context_limit * compactionOverrideWindowPercent) / 100,
});
const mockCompactionTrigger = organizationTriggerFor(MockCompactionChatModel);
const mockCompactionTriggersByOrganizationID = new Map([
	[MockChatModel.organization_id, mockCompactionTrigger],
]);
// Window smaller than mockCompactionTrigger's 16K point.
const mockSmallWindowModel: TypesGen.ChatModel = {
	...MockChatModel,
	display_name: "GPT-4o",
	context_limit: 10_000,
};

const mockModels: TypesGen.ChatModel[] = [
	{
		...MockChatModel,
		id: "model-1",
		model: "gpt-4o",
		display_name: "GPT-4o",
		is_default: true,
		context_limit: 128000,
		compression_threshold: 80,
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
	{
		...MockChatModel,
		id: "model-2",
		ai_provider_id: "provider-anthropic",
		model: "claude-sonnet",
		display_name: "Claude Sonnet",
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
	{
		...MockChatModel,
		id: "model-3",
		model: "gpt-3.5",
		display_name: "GPT-3.5 (Disabled)",
		enabled: false,
		context_limit: 16000,
		compression_threshold: 60,
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
];

const meta = {
	title: "pages/AgentsPage/UserCompactionThresholdSettings",
	component: UserCompactionThresholdSettings,
	decorators: [withAuthProvider, withDashboardProvider],
	args: {
		models: mockModels,
		providerInfoByID: new Map([
			["provider-1", { provider: "openai", displayName: "OpenAI", icon: "" }],
			[
				"provider-anthropic",
				{ provider: "anthropic", displayName: "Anthropic", icon: "" },
			],
		]),
		organizations: [modelsOrganization],
		compactionTriggersByOrganizationID: new Map(),
		compactionTriggerLoadErrors: [],
		isLoadingModels: false,
		thresholds: [],
		isThresholdsLoading: false,
		thresholdsError: undefined,
		onSaveThreshold: fn(async () => undefined),
		onResetThreshold: fn(async () => undefined),
	},
	parameters: {
		user: MockUserOwner,
	},
} satisfies Meta<typeof UserCompactionThresholdSettings>;

export default meta;
type Story = StoryObj<typeof meta>;

// The click inside userEvent.type closes the Radix tooltip, and it reopens
// only on a fresh pointer enter.
const reopenTooltip = async (element: HTMLElement) => {
	await userEvent.unhover(element);
	await userEvent.hover(element);
};

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("textbox", {
			name: /GPT-4o compaction threshold/i,
		});

		await userEvent.type(gpt4oInput, "95");
	},
};

export const ContextWindowTracksDraft: Story = {
	name: "Context Window Tracks Draft",
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("textbox", {
			name: /GPT-4o compaction threshold/i,
		});

		// 128K window: default 80% compacts at ~102K, the draft moves it to ~64K.
		await userEvent.type(gpt4oInput, "50");
	},
};

export const UnknownContextWindow: Story = {
	args: {
		models: [{ ...mockModels[0], context_limit: 0 }],
	},
};

export const ConfiguredProviderIcon: Story = {
	args: {
		providerInfoByID: new Map([
			[
				"provider-1",
				{
					provider: "openai",
					displayName: "DeepSeek",
					icon: "/icon/widgets.svg",
				},
			],
		]),
	},
};

export const EmptyOrganizationDisplayNameFallsBackToName: Story = {
	args: {
		organizations: [organizationWithEmptyDisplayName],
		thresholds: [{ model_config_id: "model-1", threshold_percent: 90 }],
	},
};

export const MultipleUnsavedRows: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			await canvas.findByRole("textbox", {
				name: /GPT-4o compaction threshold/i,
			}),
			"95",
		);
		await userEvent.type(
			await canvas.findByRole("textbox", {
				name: /Claude Sonnet compaction threshold/i,
			}),
			"50",
		);
	},
};

export const WithOverrides: Story = {
	args: {
		thresholds: [
			{ model_config_id: "model-1", threshold_percent: 90 },
			{ model_config_id: "model-2", threshold_percent: 50 },
		],
	},
};

export const InvalidDraftShowsActions: Story = {
	name: "Invalid Draft Shows Actions",
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("textbox", {
			name: /GPT-4o compaction threshold/i,
		});

		// Type an out-of-range value.
		await userEvent.type(gpt4oInput, "150");
	},
};

export const DisableCompactionWarning: Story = {
	name: "100% Disable Compaction Warning",
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("textbox", {
			name: /GPT-4o compaction threshold/i,
		});

		await userEvent.type(gpt4oInput, "100");
		await reopenTooltip(gpt4oInput);
	},
};

export const DisableCompactionWithOverridesLoadError: Story = {
	name: "100% Disable Compaction Warning With Overrides Load Error",
	args: {
		compactionTriggerLoadErrors: [
			{
				organizationID: MockChatModel.organization_id,
				error: new Error("Network Error"),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const gpt4oInput = await canvas.findByRole("textbox", {
			name: /GPT-4o compaction threshold/i,
		});

		await userEvent.type(gpt4oInput, "100");
		await reopenTooltip(gpt4oInput);
	},
};

export const OrganizationCompactionTriggerWarning: Story = {
	args: {
		compactionTriggersByOrganizationID: mockCompactionTriggersByOrganizationID,
	},
	play: async ({ canvasElement }) => {
		const row = within(canvasElement).getByRole("row", { name: /GPT-4o/i });
		await userEvent.click(
			within(row).getByRole("button", {
				name: /Organization override for GPT-4o/i,
			}),
		);
	},
};

export const OrganizationTriggerWarningAtDisabledThreshold: Story = {
	args: {
		compactionTriggersByOrganizationID: new Map([
			[
				MockChatModel.organization_id,
				organizationTriggerFor({
					...MockCompactionChatModel,
					context_limit: 160_000,
				}),
			],
		]),
	},
	play: async ({ canvasElement }) => {
		const row = within(canvasElement).getByRole("row", { name: /GPT-4o/i });
		const input = within(row).getByRole("textbox", {
			name: /GPT-4o compaction threshold/i,
		});
		await userEvent.type(input, "100");
		await reopenTooltip(input);
	},
};

export const OrganizationTriggerBeyondModelWindow: Story = {
	args: {
		models: [mockSmallWindowModel],
		thresholds: [
			{ model_config_id: mockSmallWindowModel.id, threshold_percent: 100 },
		],
		compactionTriggersByOrganizationID: mockCompactionTriggersByOrganizationID,
	},
	play: async ({ canvasElement }) => {
		const row = within(canvasElement).getByRole("row", { name: /GPT-4o/i });
		await userEvent.click(
			within(row).getByRole("button", {
				name: /Organization override for GPT-4o/i,
			}),
		);
	},
};

export const DisableCompactionBeyondOrganizationPoint: Story = {
	args: {
		models: [mockSmallWindowModel],
		compactionTriggersByOrganizationID: mockCompactionTriggersByOrganizationID,
	},
	play: async ({ canvasElement }) => {
		const row = within(canvasElement).getByRole("row", { name: /GPT-4o/i });
		const input = within(row).getByRole("textbox", {
			name: /GPT-4o compaction threshold/i,
		});
		await userEvent.type(input, "100");
		await reopenTooltip(input);
	},
};

export const CompactionTriggersLoadError: Story = {
	args: {
		models: [
			...mockModels,
			{
				...mockModels[0],
				id: "model-organization-2",
				organization_id: MockOrganization2.id,
			},
		],
		organizations: [modelsOrganization, MockOrganization2],
		compactionTriggerLoadErrors: [
			{
				organizationID: MockOrganization2.id,
				error: new Error("Network Error"),
			},
		],
	},
};

export const Loading: Story = {
	args: {
		isThresholdsLoading: true,
	},
};

export const RowSaveFailure: Story = {
	name: "Row Save Failure",
	args: {
		onSaveThreshold: fn(async (modelId: string) => {
			if (modelId === "model-2") {
				throw new globalThis.Error("Network error");
			}
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const claudeRow = within(
			canvas.getByRole("row", { name: /Claude Sonnet/i }),
		);
		await userEvent.type(claudeRow.getByRole("textbox"), "55");
		await userEvent.click(claudeRow.getByRole("button", { name: /^Save / }));
		await claudeRow.findByText("Network error");
	},
};

export const OrganizationFilter: Story = {
	args: {
		models: [
			mockModels[0],
			{
				...mockModels[1],
				organization_id: MockOrganization2.id,
			},
		],
		organizations: [modelsOrganization, MockOrganization2],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const filter = await canvas.findByRole("combobox", {
			name: `Organization ${modelsOrganization.display_name}`,
		});

		await userEvent.click(filter);
		const option = await screen.findByRole("option", {
			name: new RegExp(`${MockOrganization2.display_name}$`),
		});
		await userEvent.click(option);
	},
};

export const SingleOrganizationHidesFilter: Story = {};

export const OrganizationFilterPreservesDraft: Story = {
	args: {
		models: [
			mockModels[0],
			{
				...mockModels[1],
				organization_id: MockOrganization2.id,
			},
		],
		organizations: [modelsOrganization, MockOrganization2],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			await canvas.findByRole("textbox", {
				name: /GPT-4o compaction threshold/i,
			}),
			"95",
		);
		await userEvent.click(canvas.getByRole("combobox"));
		await userEvent.click(
			await screen.findByRole("option", {
				name: new RegExp(`${MockOrganization2.display_name}$`),
			}),
		);
		await userEvent.click(canvas.getByRole("combobox"));
		await userEvent.click(
			await screen.findByRole("option", {
				name: new RegExp(`${modelsOrganization.display_name}$`),
			}),
		);
	},
};

export const SavedRow: Story = {
	play: async ({ canvasElement }) => {
		const row = within(
			within(canvasElement).getByRole("row", { name: /GPT-4o/i }),
		);
		await userEvent.type(row.getByRole("textbox"), "90");
		await userEvent.click(row.getByRole("button", { name: /^Save / }));
		await row.findByText("Saved");
	},
};

export const ClearedOverrideDraft: Story = {
	args: {
		thresholds: [{ model_config_id: "model-1", threshold_percent: 90 }],
	},
	play: async ({ canvasElement }) => {
		const row = within(
			within(canvasElement).getByRole("row", { name: /GPT-4o/i }),
		);
		await userEvent.clear(row.getByRole("textbox"));
	},
};

export const PendingRowSave: Story = {
	args: {
		onSaveThreshold: fn(() => new Promise<void>(() => {})),
	},
	play: async ({ canvasElement }) => {
		const row = within(
			within(canvasElement).getByRole("row", { name: /GPT-4o/i }),
		);
		await userEvent.type(row.getByRole("textbox"), "90");
		await userEvent.click(row.getByRole("button", { name: /^Save / }));
	},
};

export const UnsavedLeaveDialog: Story = {
	decorators: [
		(Story) => (
			<>
				<Link to="/general">General settings</Link>
				<Story />
			</>
		),
	],
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/compaction" },
			routing: [
				{ path: "/compaction", useStoryElement: true },
				{ path: "/general", element: <h1>General settings</h1> },
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			await canvas.findByRole("textbox", {
				name: /GPT-4o compaction threshold/i,
			}),
			"95",
		);
		await userEvent.click(
			canvas.getByRole("link", { name: "General settings" }),
		);
	},
};

export const LongListNarrowViewport: Story = {
	args: {
		models: Array.from({ length: 12 }, (_, index) => ({
			...mockModels[0],
			id: `model-long-list-${index}`,
			display_name: `GPT-4o model ${index + 1}`,
		})),
	},
	globals: { viewport: { value: "ipad" } },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			await canvas.findByRole("textbox", {
				name: /GPT-4o model 1 compaction threshold/i,
			}),
			"95",
		);
	},
};

export const MobileEditedRow: Story = {
	args: {
		models: [mockModels[0]],
	},
	globals: { viewport: { value: "iphone12" } },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			await canvas.findByRole("textbox", {
				name: /GPT-4o compaction threshold/i,
			}),
			"95",
		);
	},
};

export const NoEnabledModels: Story = {
	args: {
		models: [],
	},
};

export const ErrorState: Story = {
	name: "Error",
	args: {
		thresholdsError: new globalThis.Error("Failed to load thresholds"),
	},
};

export const ModelLoadErrorWhileLoading: Story = {
	args: {
		models: [],
		isLoadingModels: true,
		modelsError: new globalThis.Error(
			"Failed to load models from one organization",
		),
	},
};

export const ModelLoadError: Story = {
	args: {
		models: [],
		isLoadingModels: false,
		modelsError: new globalThis.Error("Failed to load models"),
	},
};

export const PartialModelLoadError: Story = {
	args: {
		modelsError: new globalThis.Error(
			"Failed to load models from one organization",
		),
	},
};
