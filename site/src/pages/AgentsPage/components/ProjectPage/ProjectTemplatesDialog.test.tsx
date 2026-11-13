import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProject } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockTemplate,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	mockApiError,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { ProjectTemplatesDialog } from "./ProjectTemplatesDialog";

type DialogProps = React.ComponentProps<typeof ProjectTemplatesDialog>;

const renderDialog = (props: Partial<DialogProps> = {}) => {
	vi.spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
	vi.spyOn(API.experimental, "getChatProjects").mockResolvedValue([
		MockChatProject,
	]);
	vi.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValue([
		MockTemplateVersionParameter1,
		MockTemplateVersionParameter2,
	]);
	const queryClient = createTestQueryClient();
	const allProps: DialogProps = {
		project: MockChatProject,
		open: true,
		onOpenChange: vi.fn(),
		isSaving: false,
		error: undefined,
		onSave: vi.fn(),
		onResetError: vi.fn(),
		...props,
	};
	render(
		<QueryClientProvider client={queryClient}>
			<ProjectTemplatesDialog {...allProps} />
		</QueryClientProvider>,
	);
	return { user: userEvent.setup(), props: allProps };
};

describe("ProjectTemplatesDialog", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("clears the save error when the draft changes", async () => {
		const { user, props } = renderDialog({
			error: mockApiError({ message: "Failed to update chat project." }),
		});

		await user.click(await screen.findByRole("button", { name: "Select all" }));

		expect(props.onResetError).toHaveBeenCalled();
	});

	it("keeps a saved empty parameter value when other defaults change", async () => {
		const project: ChatProject = {
			...MockChatProject,
			templates: {
				template_ids: [MockTemplate.id],
				default_template_id: MockTemplate.id,
				default_parameters: {
					[MockTemplateVersionParameter1.name]: "",
					[MockTemplateVersionParameter2.name]: "old",
				},
			},
		};
		const { user, props } = renderDialog({ project });

		const second = await screen.findByRole("textbox", {
			name: MockTemplateVersionParameter2.name,
		});
		await user.clear(second);
		await user.type(second, "new");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(props.onSave).toHaveBeenCalledWith({
			template_ids: [MockTemplate.id],
			default_template_id: MockTemplate.id,
			default_parameters: {
				[MockTemplateVersionParameter1.name]: "",
				[MockTemplateVersionParameter2.name]: "new",
			},
		});
	});

	it("clears a default preset that is no longer in the active version", async () => {
		vi.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([]);
		const { user, props } = renderDialog({
			project: {
				...MockChatProject,
				templates: {
					template_ids: [MockTemplate.id],
					default_template_id: MockTemplate.id,
					default_preset_id: "stale-preset",
				},
			},
		});

		const save = await screen.findByRole("button", { name: "Save" });
		await waitFor(() => expect(save).toBeEnabled());
		await user.click(save);

		expect(props.onSave).toHaveBeenCalledWith({
			template_ids: [MockTemplate.id],
			default_template_id: MockTemplate.id,
		});
	});
});
