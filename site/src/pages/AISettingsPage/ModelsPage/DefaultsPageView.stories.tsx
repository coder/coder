import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	chatAIProviderCatalogKey,
	chatModelConfigsByOrganizationKey,
	chatModelOverrideKey,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatModelConfig } from "#/testHelpers/chatModels";
import { MockDefaultOrganization } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import DefaultsPage from "./DefaultsPage";
import { OrganizationModelsContext } from "./organizationModels";

const buildModelConfig = (
	overrides: Partial<TypesGen.ChatModelConfig> = {},
): TypesGen.ChatModelConfig => ({
	...MockChatModelConfig,
	organization_id: MockDefaultOrganization.id,
	...overrides,
});

const generalModelConfig = buildModelConfig({
	id: "model-general-gpt-4.1-mini",
	model: "gpt-4.1-mini",
	display_name: "GPT 4.1 Mini",
});

const claudeModelConfig = buildModelConfig({
	id: "model-claude-sonnet-4",
	ai_provider_id: "provider-anthropic",
	model: "claude-sonnet-4",
	display_name: "Claude Sonnet 4",
	context_limit: 200_000,
});

const disabledModelConfig = buildModelConfig({
	id: "model-disabled",
	model: "gpt-4.1-legacy",
	display_name: "GPT 4.1 Legacy",
	enabled: false,
});

const allModelConfigs = [
	generalModelConfig,
	claudeModelConfig,
	disabledModelConfig,
];

const providerCatalog: TypesGen.ChatAIProviderCatalogEntry[] = [
	{
		id: "provider-1",
		type: "openai",
		display_name: "OpenAI",
		icon: "",
		enabled: true,
		has_api_key: true,
		has_user_api_key: false,
		allow_user_api_key: true,
	},
	{
		id: "provider-anthropic",
		type: "anthropic",
		display_name: "Anthropic",
		icon: "",
		enabled: true,
		has_api_key: true,
		has_user_api_key: false,
		allow_user_api_key: true,
	},
];

const buildOverrideResponse = (
	context: TypesGen.ChatModelOverrideContext,
	overrides: Partial<TypesGen.ChatModelOverrideResponse> = {},
): TypesGen.ChatModelOverrideResponse => ({
	context,
	model_config_id: "",
	...overrides,
});

const buildOverrideQueries = ({
	general = buildOverrideResponse("general"),
	titleGeneration = buildOverrideResponse("title_generation"),
	compaction = buildOverrideResponse("compaction"),
	explore = buildOverrideResponse("explore"),
	advisor = buildOverrideResponse("advisor"),
	modelConfigs = allModelConfigs,
}: {
	general?: TypesGen.ChatModelOverrideResponse;
	titleGeneration?: TypesGen.ChatModelOverrideResponse;
	compaction?: TypesGen.ChatModelOverrideResponse;
	explore?: TypesGen.ChatModelOverrideResponse;
	advisor?: TypesGen.ChatModelOverrideResponse;
	modelConfigs?: TypesGen.ChatModelConfig[];
} = {}) => [
	{
		key: chatModelConfigsByOrganizationKey(MockDefaultOrganization.id),
		data: modelConfigs,
	},
	{ key: chatAIProviderCatalogKey, data: providerCatalog },
	{
		key: chatModelOverrideKey(MockDefaultOrganization.id, "general"),
		data: general,
	},
	{
		key: chatModelOverrideKey(MockDefaultOrganization.id, "title_generation"),
		data: titleGeneration,
	},
	{
		key: chatModelOverrideKey(MockDefaultOrganization.id, "compaction"),
		data: compaction,
	},
	{
		key: chatModelOverrideKey(MockDefaultOrganization.id, "explore"),
		data: explore,
	},
	{
		key: chatModelOverrideKey(MockDefaultOrganization.id, "advisor"),
		data: advisor,
	},
];

const getSection = async (
	canvasElement: HTMLElement,
	headingName: string,
): Promise<HTMLElement> => {
	const canvas = within(canvasElement);
	const heading = await canvas.findByRole("heading", { name: headingName });
	const setting = heading.closest("form");
	if (!(setting instanceof HTMLElement)) {
		throw new Error(`Expected ${headingName} heading to live inside a form.`);
	}
	return setting;
};

const meta = {
	title: "pages/AISettingsPage/ModelsPage/DefaultsPage",
	component: DefaultsPage,
	decorators: [
		withDashboardProvider,
		(Story) => (
			<OrganizationModelsContext.Provider
				value={{
					organization: MockDefaultOrganization,
					organizations: [MockDefaultOrganization],
				}}
			>
				<Story />
			</OrganizationModelsContext.Provider>
		),
	],
	parameters: {
		// TODO: Stories in this file fail when pixel runs their play functions. Fix them and remove the exclude.
		pixel: { exclude: true },
		experiments: ["chat-advisor"],
		queries: buildOverrideQueries(),
		reactRouter: reactRouterParameters({
			location: {
				path: `/ai/settings/organizations/${MockDefaultOrganization.name}/defaults`,
			},
			routing: [
				{
					path: "/ai/settings/organizations/:organization/defaults",
					useStoryElement: true,
				},
			],
		}),
	},
} satisfies Meta<typeof DefaultsPage>;

export default meta;
type Story = StoryObj<typeof DefaultsPage>;

export const AllUnset: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("heading", { name: "Defaults & overrides" });

		const headings = await canvas.findAllByRole("heading", { level: 3 });
		expect(headings.map((heading) => heading.textContent?.trim())).toEqual([
			"General model",
			"Title generation model",
			"Compaction model",
			"Explore subagent model",
			"Advisor model",
		]);

		const unsetSections = [
			{ headingName: "General model", placeholder: "Use chat default" },
			{
				headingName: "Title generation model",
				placeholder: "Use title default",
			},
			{ headingName: "Compaction model", placeholder: "Use chat model" },
			{
				headingName: "Explore subagent model",
				placeholder: "Use chat default",
			},
			{ headingName: "Advisor model", placeholder: "Reuse chat model" },
		];
		for (const { headingName, placeholder } of unsetSections) {
			const section = await getSection(canvasElement, headingName);
			expect(
				within(section).getByRole("combobox", { name: placeholder }),
			).toBeInTheDocument();
			expect(
				within(section).queryByRole("button", { name: "Save" }),
			).not.toBeInTheDocument();
		}
	},
};

export const WithSavedValues: Story = {
	parameters: {
		queries: buildOverrideQueries({
			general: buildOverrideResponse("general", {
				model_config_id: generalModelConfig.id,
			}),
			advisor: buildOverrideResponse("advisor", {
				model_config_id: claudeModelConfig.id,
			}),
		}),
	},
	play: async ({ canvasElement }) => {
		const generalSection = await getSection(canvasElement, "General model");
		expect(
			within(generalSection).getByRole("combobox", {
				name: /gpt 4\.1 mini/i,
			}),
		).toBeInTheDocument();

		const advisorSection = await getSection(canvasElement, "Advisor model");
		expect(
			within(advisorSection).getByRole("combobox", {
				name: /claude sonnet 4/i,
			}),
		).toBeInTheDocument();
	},
};

export const AdvisorHiddenWithoutExperiment: Story = {
	parameters: {
		experiments: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("heading", { name: "General model" });
		expect(
			canvas.queryByRole("heading", { name: "Advisor model" }),
		).not.toBeInTheDocument();
	},
};

export const SaveAdvisorOverride: Story = {
	beforeEach: () => {
		spyOn(
			API.experimental,
			"updateOrganizationChatModelOverride",
		).mockResolvedValue();
	},
	play: async ({ canvasElement }) => {
		const advisorSection = await getSection(canvasElement, "Advisor model");
		const trigger = within(advisorSection).getByRole("combobox", {
			name: "Reuse chat model",
		});
		await userEvent.click(trigger);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("option", { name: /Claude Sonnet 4/ }),
		);
		const saveButton = within(advisorSection).getByRole("button", {
			name: "Save",
		});
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});
		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(
				API.experimental.updateOrganizationChatModelOverride,
			).toHaveBeenCalledWith(MockDefaultOrganization.id, "advisor", {
				model_config_id: claudeModelConfig.id,
			});
		});
	},
};
