import {
	act,
	fireEvent,
	render as renderWithProviders,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { API } from "#/api/api";
import {
	organizationChatModelOverrides,
	organizationChatModelsKey,
} from "#/api/queries/chats";
import type { ChatModel } from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { MockDefaultOrganization, mockApiError } from "#/testHelpers/entities";
import { createTestQueryClient, render } from "#/testHelpers/renderHelpers";
import { OrganizationAgentSettings } from "./OrganizationAgentSettings";

const mockDefaultModel: ChatModel = {
	...MockChatModel,
	organization_id: MockDefaultOrganization.id,
	is_default: true,
};
const mockAlternateModel: ChatModel = {
	...MockChatModel,
	organization_id: MockDefaultOrganization.id,
	id: "model-2",
	model: "gpt-5-mini",
	display_name: "GPT-5 Mini",
};
const mockThirdModel: ChatModel = {
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
		.mockResolvedValue({ ...mockAlternateModel, is_default: true });
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

// Submitting the form reports the selection as the request payload, so the
// assertion does not depend on how the picker renders its label.
const expectSubmitSaves = async (
	form: HTMLElement,
	updateChatModel: ReturnType<typeof mockOverridesAndUpdate>,
	model: ChatModel,
) => {
	const callsBefore = updateChatModel.mock.calls.length;
	fireEvent.submit(form);
	await waitFor(() =>
		expect(updateChatModel).toHaveBeenCalledTimes(callsBefore + 1),
	);
	expect(updateChatModel).toHaveBeenLastCalledWith(
		MockDefaultOrganization.id,
		model.id,
		{ is_default: true },
	);
};

const refetchCatalog = async (
	queryClient: ReturnType<typeof createTestQueryClient>,
	getChatModels: { mock: { calls: unknown[] } },
	expectedCalls = 2,
) => {
	await act(() =>
		queryClient.invalidateQueries({
			queryKey: organizationChatModelsKey(MockDefaultOrganization.id),
		}),
	);
	await waitFor(() =>
		expect(getChatModels.mock.calls).toHaveLength(expectedCalls),
	);
};

describe("OrganizationAgentSettings", () => {
	it("promotes the selected model to the organization default", async () => {
		vi.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(
				chatModelsResponse([
					mockDefaultModel,
					mockAlternateModel,
					mockThirdModel,
				]),
			)
			.mockResolvedValue(
				chatModelsResponse([
					{ ...mockDefaultModel, is_default: false },
					{ ...mockAlternateModel, is_default: true },
					mockThirdModel,
				]),
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
			mockDefaultModel,
			mockAlternateModel,
		);
		await user.click(
			await within(defaultSection).findByRole("button", { name: "Save" }),
		);

		await waitFor(() => {
			expect(updateChatModel).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				mockAlternateModel.id,
				{ is_default: true },
			);
		});

		// A new pick while the saved indicator is still showing must be savable.
		await within(defaultSection).findByText("Saved");
		await selectModel(user, mockAlternateModel, mockThirdModel);
		await user.click(
			await within(defaultSection).findByRole("button", { name: "Save" }),
		);
		await waitFor(() => {
			expect(updateChatModel).toHaveBeenLastCalledWith(
				MockDefaultOrganization.id,
				mockThirdModel.id,
				{ is_default: true },
			);
		});
	});

	it("keeps the saved model when the catalog refresh fails", async () => {
		vi.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(
				chatModelsResponse([mockDefaultModel, mockAlternateModel]),
			)
			.mockRejectedValue(new Error("catalog unavailable"));
		const updateChatModel = mockOverridesAndUpdate();
		const user = userEvent.setup();
		renderWithQueryClient();

		const defaultSection = await selectModel(
			user,
			mockDefaultModel,
			mockAlternateModel,
		);
		await user.click(
			await within(defaultSection).findByRole("button", { name: "Save" }),
		);
		// The saved indicator marks the end of the mutation success path, which
		// includes the failed catalog refetch.
		await within(defaultSection).findByText("Saved");

		await expectSubmitSaves(
			defaultSection,
			updateChatModel,
			mockAlternateModel,
		);
	});

	it("keeps an unsaved selection when the model catalog refetches", async () => {
		const getChatModels = vi
			.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(
				chatModelsResponse([
					mockDefaultModel,
					mockAlternateModel,
					mockThirdModel,
				]),
			)
			.mockResolvedValue(
				chatModelsResponse([
					{ ...mockDefaultModel, is_default: false },
					mockAlternateModel,
					{ ...mockThirdModel, is_default: true },
				]),
			);
		const updateChatModel = mockOverridesAndUpdate();
		const user = userEvent.setup();
		const queryClient = renderWithQueryClient();

		const defaultSection = await selectModel(
			user,
			mockDefaultModel,
			mockAlternateModel,
		);
		await refetchCatalog(queryClient, getChatModels);

		await user.click(
			await within(defaultSection).findByRole("button", { name: "Save" }),
		);

		await waitFor(() => {
			expect(updateChatModel).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				mockAlternateModel.id,
				{ is_default: true },
			);
		});
	});

	it("drops an unsaved selection the refetched catalog no longer lists", async () => {
		const getChatModels = vi
			.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(
				chatModelsResponse([mockDefaultModel, mockAlternateModel]),
			)
			.mockResolvedValueOnce(
				chatModelsResponse([
					mockDefaultModel,
					{ ...mockAlternateModel, enabled: false },
				]),
			)
			.mockResolvedValue(
				chatModelsResponse([mockDefaultModel, mockAlternateModel]),
			);
		const updateChatModel = mockOverridesAndUpdate();
		const user = userEvent.setup();
		const queryClient = renderWithQueryClient();

		const defaultSection = await selectModel(
			user,
			mockDefaultModel,
			mockAlternateModel,
		);
		await refetchCatalog(queryClient, getChatModels);
		// Relisting the dropped model must not resurrect the selection.
		await refetchCatalog(queryClient, getChatModels, 3);

		await expectSubmitSaves(defaultSection, updateChatModel, mockDefaultModel);
	});

	it("follows a refetched default after the saved model is reselected", async () => {
		const getChatModels = vi
			.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(
				chatModelsResponse([
					mockDefaultModel,
					mockAlternateModel,
					mockThirdModel,
				]),
			)
			.mockResolvedValue(
				chatModelsResponse([
					{ ...mockDefaultModel, is_default: false },
					mockAlternateModel,
					{ ...mockThirdModel, is_default: true },
				]),
			);
		const updateChatModel = mockOverridesAndUpdate();
		const user = userEvent.setup();
		const queryClient = renderWithQueryClient();

		const defaultSection = await selectModel(
			user,
			mockDefaultModel,
			mockAlternateModel,
		);
		await selectModel(user, mockAlternateModel, mockDefaultModel);
		await refetchCatalog(queryClient, getChatModels);

		await expectSubmitSaves(defaultSection, updateChatModel, mockThirdModel);
	});

	it("clears a pending selection once the server adopts it", async () => {
		const getChatModels = vi
			.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(
				chatModelsResponse([
					mockDefaultModel,
					mockAlternateModel,
					mockThirdModel,
				]),
			)
			.mockResolvedValueOnce(
				chatModelsResponse([
					{ ...mockDefaultModel, is_default: false },
					{ ...mockAlternateModel, is_default: true },
					mockThirdModel,
				]),
			)
			.mockResolvedValue(
				chatModelsResponse([
					{ ...mockDefaultModel, is_default: false },
					mockAlternateModel,
					{ ...mockThirdModel, is_default: true },
				]),
			);
		const updateChatModel = mockOverridesAndUpdate();
		const user = userEvent.setup();
		const queryClient = renderWithQueryClient();

		const defaultSection = await selectModel(
			user,
			mockDefaultModel,
			mockAlternateModel,
		);
		await refetchCatalog(queryClient, getChatModels);
		await refetchCatalog(queryClient, getChatModels, 3);

		await expectSubmitSaves(defaultSection, updateChatModel, mockThirdModel);
	});

	it("keeps the empty-catalog status when the overrides refresh fails", async () => {
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue(
			chatModelsResponse([]),
		);
		const getOverrides = vi
			.spyOn(API.experimental, "getOrganizationChatModelOverrides")
			.mockResolvedValueOnce({ overrides: [] })
			.mockRejectedValue(mockApiError({ message: "overrides unavailable" }));
		const queryClient = renderWithQueryClient();

		await screen.findByText("This organization has no enabled chat models.");
		await act(() =>
			queryClient.invalidateQueries({
				queryKey: organizationChatModelOverrides(MockDefaultOrganization.id)
					.queryKey,
			}),
		);
		await waitFor(() => expect(getOverrides).toHaveBeenCalledTimes(2));
		await screen.findByText("overrides unavailable");

		expect(
			screen.getByText("This organization has no enabled chat models."),
		).toBeInTheDocument();
	});
});
