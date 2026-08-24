import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import {
	MockDefaultOrganization,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { OrganizationModelsContext } from "../organizationModels";
import DefaultsPage from "./DefaultsPage";

const meta: Meta<typeof DefaultsPage> = {
	title: "pages/AISettingsPage/ModelsPage/DefaultsPage",
	component: DefaultsPage,
	decorators: [
		(Story) => (
			<OrganizationModelsContext.Provider
				value={{
					organization: MockDefaultOrganization,
					organizations: [MockDefaultOrganization],
					permissions: MockOrganizationPermissions,
					requestedOrganizationDenied: false,
				}}
			>
				<Story />
			</OrganizationModelsContext.Provider>
		),
		withDashboardProvider,
	],
};
export default meta;
type Story = StoryObj<typeof DefaultsPage>;

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
