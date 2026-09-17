import {
	act,
	render as renderWithProviders,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { API } from "#/api/api";
import { organizationChatModelsKey } from "#/api/queries/chats";
import type { ChatModel } from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { MockDefaultOrganization } from "#/testHelpers/entities";
import { createTestQueryClient, render } from "#/testHelpers/renderHelpers";
import { OrganizationAgentSettings } from "./OrganizationAgentSettings";

const defaultModel: ChatModel = {
	...MockChatModel,
	organization_id: MockDefaultOrganization.id,
	is_default: true,
};
const alternateModel: ChatModel = {
	...MockChatModel,
	organization_id: MockDefaultOrganization.id,
	id: "model-2",
	model: "gpt-5-mini",
	display_name: "GPT-5 Mini",
};
const thirdModel: ChatModel = {
	...MockChatModel,
	organization_id: MockDefaultOrganization.id,
	id: "model-3",
	model: "gpt-5-nano",
	display_name: "GPT-5 Nano",
};

const chatModelsResponse = (models: ChatModel[]) => ({
	models,
	providers: [MockChatModelProviderDescriptor],
	unsupported_providers: [],
});

const selectAlternateModel = async (
	user: ReturnType<typeof userEvent.setup>,
) => {
	const defaultSection = await screen.findByRole("form", {
		name: "Default model",
	});
	await user.click(
		await within(defaultSection).findByRole("combobox", {
			name: `Default model, ${defaultModel.display_name}`,
		}),
	);
	await user.click(
		await screen.findByRole("option", {
			name: new RegExp(alternateModel.display_name),
		}),
	);
	return defaultSection;
};

describe("OrganizationAgentSettings", () => {
	it("promotes the selected model to the organization default", async () => {
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue(
			chatModelsResponse([defaultModel, alternateModel]),
		);
		vi.spyOn(
			API.experimental,
			"getOrganizationChatModelOverrides",
		).mockResolvedValue({ overrides: [] });
		const updateChatModel = vi
			.spyOn(API.experimental, "updateChatModel")
			.mockResolvedValue({ ...alternateModel, is_default: true });
		const user = userEvent.setup();

		render(
			<OrganizationAgentSettings
				organization={MockDefaultOrganization}
				canEdit
				showAdvisor
			/>,
		);

		const defaultSection = await selectAlternateModel(user);
		await user.click(
			await within(defaultSection).findByRole("button", { name: "Save" }),
		);

		await waitFor(() => {
			expect(updateChatModel).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				alternateModel.id,
				{ is_default: true },
			);
		});
	});

	it("keeps an unsaved selection when the model catalog refetches", async () => {
		const getChatModels = vi
			.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(
				chatModelsResponse([defaultModel, alternateModel, thirdModel]),
			)
			.mockResolvedValue(
				chatModelsResponse([
					{ ...defaultModel, is_default: false },
					alternateModel,
					{ ...thirdModel, is_default: true },
				]),
			);
		vi.spyOn(
			API.experimental,
			"getOrganizationChatModelOverrides",
		).mockResolvedValue({ overrides: [] });
		const updateChatModel = vi
			.spyOn(API.experimental, "updateChatModel")
			.mockResolvedValue({ ...alternateModel, is_default: true });
		const queryClient = createTestQueryClient();
		const user = userEvent.setup();

		renderWithProviders(
			<AppProviders queryClient={queryClient}>
				<OrganizationAgentSettings
					organization={MockDefaultOrganization}
					canEdit
					showAdvisor
				/>
			</AppProviders>,
		);

		const defaultSection = await selectAlternateModel(user);
		await act(() =>
			queryClient.invalidateQueries({
				queryKey: organizationChatModelsKey(MockDefaultOrganization.id),
			}),
		);
		await waitFor(() => expect(getChatModels).toHaveBeenCalledTimes(2));

		await user.click(
			await within(defaultSection).findByRole("button", { name: "Save" }),
		);

		await waitFor(() => {
			expect(updateChatModel).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				alternateModel.id,
				{ is_default: true },
			);
		});
	});
});
