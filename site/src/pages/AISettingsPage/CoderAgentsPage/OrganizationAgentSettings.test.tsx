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

const defaultSectionName = { name: "Default model" };
const pickerName = (model: ChatModel) => ({
	name: `Default model, ${model.display_name}`,
});

const selectModel = async (
	user: ReturnType<typeof userEvent.setup>,
	from: ChatModel,
	to: ChatModel,
) => {
	const defaultSection = await screen.findByRole("form", defaultSectionName);
	await user.click(
		await within(defaultSection).findByRole("combobox", pickerName(from)),
	);
	await user.click(
		await screen.findByRole("option", { name: new RegExp(to.display_name) }),
	);
	return defaultSection;
};

const mockOverridesAndUpdate = () => {
	vi.spyOn(
		API.experimental,
		"getOrganizationChatModelOverrides",
	).mockResolvedValue({ overrides: [] });
	return vi
		.spyOn(API.experimental, "updateChatModel")
		.mockResolvedValue({ ...alternateModel, is_default: true });
};

const renderWithQueryClient = () => {
	const queryClient = createTestQueryClient();
	renderWithProviders(
		<AppProviders queryClient={queryClient}>
			<OrganizationAgentSettings
				organization={MockDefaultOrganization}
				canEdit
				showAdvisor
			/>
		</AppProviders>,
	);
	return queryClient;
};

const refetchCatalog = async (
	queryClient: ReturnType<typeof createTestQueryClient>,
	getChatModels: { mock: { calls: unknown[] } },
) => {
	await act(() =>
		queryClient.invalidateQueries({
			queryKey: organizationChatModelsKey(MockDefaultOrganization.id),
		}),
	);
	await waitFor(() => expect(getChatModels.mock.calls).toHaveLength(2));
};

describe("OrganizationAgentSettings", () => {
	it("promotes the selected model to the organization default", async () => {
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue(
			chatModelsResponse([defaultModel, alternateModel]),
		);
		const updateChatModel = mockOverridesAndUpdate();
		const user = userEvent.setup();

		render(
			<OrganizationAgentSettings
				organization={MockDefaultOrganization}
				canEdit
				showAdvisor
			/>,
		);

		const defaultSection = await selectModel(
			user,
			defaultModel,
			alternateModel,
		);
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
		const updateChatModel = mockOverridesAndUpdate();
		const user = userEvent.setup();
		const queryClient = renderWithQueryClient();

		const defaultSection = await selectModel(
			user,
			defaultModel,
			alternateModel,
		);
		await refetchCatalog(queryClient, getChatModels);

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

	it("drops an unsaved selection the refetched catalog no longer lists", async () => {
		const getChatModels = vi
			.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(chatModelsResponse([defaultModel, alternateModel]))
			.mockResolvedValue(
				chatModelsResponse([
					defaultModel,
					{ ...alternateModel, enabled: false },
				]),
			);
		mockOverridesAndUpdate();
		const user = userEvent.setup();
		const queryClient = renderWithQueryClient();

		const defaultSection = await selectModel(
			user,
			defaultModel,
			alternateModel,
		);
		await refetchCatalog(queryClient, getChatModels);

		await within(defaultSection).findByRole(
			"combobox",
			pickerName(defaultModel),
		);
	});

	it("follows a refetched default after the saved model is reselected", async () => {
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
		mockOverridesAndUpdate();
		const user = userEvent.setup();
		const queryClient = renderWithQueryClient();

		const defaultSection = await selectModel(
			user,
			defaultModel,
			alternateModel,
		);
		await selectModel(user, alternateModel, defaultModel);
		await refetchCatalog(queryClient, getChatModels);

		await within(defaultSection).findByRole("combobox", pickerName(thirdModel));
	});
});
