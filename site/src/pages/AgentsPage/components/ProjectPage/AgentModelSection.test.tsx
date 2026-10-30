import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProject } from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
	MockCompactionChatModel,
} from "#/testHelpers/chatModels";
import { MockChatProject } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { AgentModelSection } from "./AgentModelSection";

const mockModelCatalog = {
	models: [
		{
			...MockChatModel,
			organization_id: MockChatProject.organization_id,
			is_default: true,
		},
		{
			...MockCompactionChatModel,
			organization_id: MockChatProject.organization_id,
		},
	],
	providers: [MockChatModelProviderDescriptor],
	unsupported_providers: [],
};

describe("AgentModelSection", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	const renderSection = (project: ChatProject = MockChatProject) => {
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue(
			mockModelCatalog,
		);
		const updateProject = vi
			.spyOn(API.experimental, "updateChatProject")
			.mockResolvedValue(project);
		render(<AgentModelSection project={project} />);
		return { updateProject, user: userEvent.setup() };
	};

	it("sets the model as soon as it is picked", async () => {
		const { updateProject, user } = renderSection();

		await user.click(
			await screen.findByRole("combobox", { name: /Agent model/ }),
		);
		await user.click(
			await screen.findByRole("option", {
				name: new RegExp(MockCompactionChatModel.display_name),
			}),
		);

		await waitFor(() =>
			expect(updateProject).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{ model_config_id: MockCompactionChatModel.id },
			),
		);
	});

	it("clears the model with the default model option", async () => {
		const { updateProject, user } = renderSection({
			...MockChatProject,
			model_config_id: MockCompactionChatModel.id,
			model_status: "available",
			model_display_name: MockCompactionChatModel.display_name,
		});

		await user.click(
			await screen.findByRole("combobox", { name: /Agent model/ }),
		);
		await user.click(
			await screen.findByRole("option", { name: /^Default model/ }),
		);

		await waitFor(() =>
			expect(updateProject).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{ model_config_id: "" },
			),
		);
	});
});
