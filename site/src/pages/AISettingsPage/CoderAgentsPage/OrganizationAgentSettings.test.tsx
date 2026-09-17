import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatModel } from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { MockDefaultOrganization } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
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

describe("OrganizationAgentSettings", () => {
	it("promotes the selected model to the organization default", async () => {
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
			models: [defaultModel, alternateModel],
			providers: [MockChatModelProviderDescriptor],
			unsupported_providers: [],
		});
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

		const defaultSection = await screen.findByRole("form", {
			name: "Default model",
		});
		await user.click(
			await within(defaultSection).findByRole("combobox", {
				name: defaultModel.display_name,
			}),
		);
		await user.click(
			await screen.findByRole("option", {
				name: new RegExp(alternateModel.display_name),
			}),
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
});
