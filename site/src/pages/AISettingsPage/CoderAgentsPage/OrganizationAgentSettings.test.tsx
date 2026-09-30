import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import escapeRegExp from "lodash/escapeRegExp";
import type { QueryClient } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { organizationChatModelsKey } from "#/api/queries/chats";
import type {
	ChatModel,
	OrganizationChatModelsResponse,
} from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { MockDefaultOrganization } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
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
const mockChatModelsResponse: OrganizationChatModelsResponse = {
	models: [mockDefaultModel, mockAlternateModel, mockThirdModel],
	providers: [MockChatModelProviderDescriptor],
	unsupported_providers: [],
};

const renderSettings = () =>
	render(
		<OrganizationAgentSettings
			organization={MockDefaultOrganization}
			canEdit
			showAdvisor
		/>,
	);

const selectModel = async (
	user: ReturnType<typeof userEvent.setup>,
	from: ChatModel,
	to: ChatModel,
) => {
	const defaultSection = await screen.findByRole("form", {
		name: "Default model",
	});
	await user.click(
		await within(defaultSection).findByRole("combobox", {
			name: `Default model, ${from.display_name}`,
		}),
	);
	await user.click(
		await screen.findByRole("option", {
			name: new RegExp(`^${escapeRegExp(to.display_name)}(\\s*\\(.+\\))?$`),
		}),
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

// A pristine row renders no Save button, so the picker's accessible name is
// the only observable of its selection after a refetch.
const expectSelectedModel = (form: HTMLElement, model: ChatModel) =>
	waitFor(() =>
		expect(within(form).getByRole("combobox")).toHaveAccessibleName(
			`Default model, ${model.display_name}`,
		),
	);

const refetchCatalog = async (queryClient: QueryClient) => {
	const getChatModels = vi.mocked(API.experimental.getChatModels);
	const callsBefore = getChatModels.mock.calls.length;
	await act(() =>
		queryClient.invalidateQueries({
			queryKey: organizationChatModelsKey(MockDefaultOrganization.id),
		}),
	);
	await waitFor(() =>
		expect(getChatModels).toHaveBeenCalledTimes(callsBefore + 1),
	);
};

describe("OrganizationAgentSettings", () => {
	it("promotes the selected model to the organization default", async () => {
		vi.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(mockChatModelsResponse)
			.mockResolvedValue({
				...mockChatModelsResponse,
				models: [
					{ ...mockDefaultModel, is_default: false },
					{ ...mockAlternateModel, is_default: true },
					mockThirdModel,
				],
			});
		const updateChatModel = mockOverridesAndUpdate();
		const user = userEvent.setup();
		renderSettings();

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
		updateChatModel.mockResolvedValueOnce({
			...mockThirdModel,
			is_default: true,
		});
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
			.mockResolvedValueOnce({
				...mockChatModelsResponse,
				models: [mockDefaultModel, mockAlternateModel],
			})
			.mockRejectedValue(new Error("catalog unavailable"));
		mockOverridesAndUpdate();
		const user = userEvent.setup();
		renderSettings();

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

		await expectSelectedModel(defaultSection, mockAlternateModel);
	});

	it("keeps an unsaved selection when the model catalog refetches", async () => {
		vi.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(mockChatModelsResponse)
			.mockResolvedValue({
				...mockChatModelsResponse,
				models: [
					{ ...mockDefaultModel, is_default: false },
					mockAlternateModel,
					{ ...mockThirdModel, is_default: true },
				],
			});
		const updateChatModel = mockOverridesAndUpdate();
		const user = userEvent.setup();
		const { queryClient } = renderSettings();

		const defaultSection = await selectModel(
			user,
			mockDefaultModel,
			mockAlternateModel,
		);
		await refetchCatalog(queryClient);

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
		vi.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce({
				...mockChatModelsResponse,
				models: [mockDefaultModel, mockAlternateModel],
			})
			.mockResolvedValueOnce({
				...mockChatModelsResponse,
				models: [mockDefaultModel, { ...mockAlternateModel, enabled: false }],
			})
			.mockResolvedValue({
				...mockChatModelsResponse,
				models: [mockDefaultModel, mockAlternateModel],
			});
		mockOverridesAndUpdate();
		const user = userEvent.setup();
		const { queryClient } = renderSettings();

		const defaultSection = await selectModel(
			user,
			mockDefaultModel,
			mockAlternateModel,
		);
		await refetchCatalog(queryClient);
		// Relisting the dropped model must not resurrect the selection.
		await refetchCatalog(queryClient);

		await expectSelectedModel(defaultSection, mockDefaultModel);
	});

	it("follows a refetched default after the saved model is reselected", async () => {
		vi.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(mockChatModelsResponse)
			.mockResolvedValue({
				...mockChatModelsResponse,
				models: [
					{ ...mockDefaultModel, is_default: false },
					mockAlternateModel,
					{ ...mockThirdModel, is_default: true },
				],
			});
		mockOverridesAndUpdate();
		const user = userEvent.setup();
		const { queryClient } = renderSettings();

		const defaultSection = await selectModel(
			user,
			mockDefaultModel,
			mockAlternateModel,
		);
		await selectModel(user, mockAlternateModel, mockDefaultModel);
		await refetchCatalog(queryClient);

		await expectSelectedModel(defaultSection, mockThirdModel);
	});

	it("clears a pending selection once the server adopts it", async () => {
		vi.spyOn(API.experimental, "getChatModels")
			.mockResolvedValueOnce(mockChatModelsResponse)
			.mockResolvedValueOnce({
				...mockChatModelsResponse,
				models: [
					{ ...mockDefaultModel, is_default: false },
					{ ...mockAlternateModel, is_default: true },
					mockThirdModel,
				],
			})
			.mockResolvedValue({
				...mockChatModelsResponse,
				models: [
					{ ...mockDefaultModel, is_default: false },
					mockAlternateModel,
					{ ...mockThirdModel, is_default: true },
				],
			});
		mockOverridesAndUpdate();
		const user = userEvent.setup();
		const { queryClient } = renderSettings();

		const defaultSection = await selectModel(
			user,
			mockDefaultModel,
			mockAlternateModel,
		);
		await refetchCatalog(queryClient);
		await refetchCatalog(queryClient);

		await expectSelectedModel(defaultSection, mockThirdModel);
	});
});
