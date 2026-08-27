import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { API } from "#/api/api";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { MockDefaultOrganization } from "#/testHelpers/entities";
import { OrganizationAgentSettings } from "./OrganizationAgentSettings";

const meta: Meta<typeof OrganizationAgentSettings> = {
	title: "pages/AISettingsPage/CoderAgentsPage/OrganizationAgentSettings",
	component: OrganizationAgentSettings,
	args: {
		organization: MockDefaultOrganization,
		canEdit: true,
		showAdvisor: true,
	},
};
export default meta;
type Story = StoryObj<typeof OrganizationAgentSettings>;

export const ClearableWhenModelCatalogFails: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatModels").mockRejectedValue(
			new Error("failed to load models"),
		);
		spyOn(
			API.experimental,
			"getOrganizationChatModelOverrides",
		).mockResolvedValue({
			overrides: [{ context: "general", model_config_id: "model-gone" }],
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// A failed model catalog fetch must not hide the override rows: an
		// admin must still be able to clear a stale override without it.
		await canvas.findAllByText(/failed to load models/i);
		const generalSection = await canvas.findByRole("form", {
			name: "General subagent",
		});
		const clear = within(generalSection).getByRole("button", {
			name: "Clear",
		});
		await expect(clear).toBeEnabled();
		await userEvent.click(clear);
		await waitFor(() =>
			expect(
				within(generalSection).getByRole("button", { name: "Save" }),
			).toBeEnabled(),
		);
	},
};

export const SavesOverrideForSelectedOrganization: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatModels").mockResolvedValue({
			models: [
				{
					...MockChatModel,
					organization_id: MockDefaultOrganization.id,
				},
			],
			providers: [MockChatModelProviderDescriptor],
			unsupported_providers: [],
		});
		spyOn(
			API.experimental,
			"getOrganizationChatModelOverrides",
		).mockResolvedValue({ overrides: [] });
		spyOn(
			API.experimental,
			"updateOrganizationChatModelOverride",
		).mockResolvedValue({
			context: "explore",
			model_config_id: MockChatModel.id,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const exploreSection = await canvas.findByRole("form", {
			name: "Explore subagent",
		});
		await userEvent.click(
			within(exploreSection).getByRole("combobox", { name: "Use default" }),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: new RegExp(MockChatModel.model),
			}),
		);
		const save = within(exploreSection).getByRole("button", { name: "Save" });
		await waitFor(() => expect(save).toBeEnabled());
		await userEvent.click(save);
		await waitFor(() => {
			expect(
				API.experimental.updateOrganizationChatModelOverride,
			).toHaveBeenCalledWith(MockDefaultOrganization.id, "explore", {
				model_config_id: MockChatModel.id,
			});
		});
	},
};
