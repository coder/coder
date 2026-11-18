import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProject } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockPreviewParameter1,
	MockPreviewParameter2,
	MockTemplate,
	mockApiError,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { NIL_UUID } from "../../utils/modelOptions";
import { ProjectTemplatesDialog } from "./ProjectTemplatesDialog";

vi.mock("#/hooks/useAuthenticated", async () => {
	const { MockUserOwner } = await import("#/testHelpers/entities");
	return {
		useAuthenticated: () => ({ user: MockUserOwner, permissions: {} }),
	};
});

type DialogProps = React.ComponentProps<typeof ProjectTemplatesDialog>;

/** A project whose only template is its default template. */
const mockProjectWithDefault: ChatProject = {
	...MockChatProject,
	templates: [MockTemplate],
	default_template: { template_id: MockTemplate.id },
};

const renderDialog = (props: Partial<DialogProps> = {}) => {
	vi.spyOn(API, "getTemplates").mockResolvedValue([MockTemplate]);
	vi.spyOn(API, "getTemplateVersionDynamicParameters").mockResolvedValue({
		id: 0,
		diagnostics: [],
		parameters: [MockPreviewParameter1, MockPreviewParameter2],
	});
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
	return {
		user: userEvent.setup(),
		props: allProps,
	};
};

const findParameterInput = async (name: string) =>
	within(await screen.findByRole("group", { name })).getByRole("textbox");

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
		const { user, props } = renderDialog({
			project: {
				...mockProjectWithDefault,
				default_template: {
					template_id: MockTemplate.id,
					parameters: {
						[MockPreviewParameter1.name]: "",
						[MockPreviewParameter2.name]: "old",
					},
				},
			},
		});

		const second = await findParameterInput(MockPreviewParameter2.display_name);
		await user.clear(second);
		await user.type(second, "new");
		await user.click(screen.getByRole("button", { name: /Save/ }));

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				attach: [],
				detach: [],
				request: {
					default_template: {
						template_id: MockTemplate.id,
						parameters: {
							[MockPreviewParameter1.name]: "",
							[MockPreviewParameter2.name]: "new",
						},
					},
				},
			}),
		);
	});

	it("clears the default template with a nil template ID", async () => {
		vi.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([]);
		const { user, props } = renderDialog({ project: mockProjectWithDefault });

		await user.click(
			await screen.findByRole("combobox", { name: "Default template" }),
		);
		await user.click(screen.getByRole("option", { name: "None" }));
		await user.click(screen.getByRole("button", { name: /Save/ }));

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				attach: [],
				detach: [],
				request: { default_template: { template_id: NIL_UUID } },
			}),
		);
	});

	const mockProjectWithRemovedParameter = {
		...mockProjectWithDefault,
		default_template: {
			template_id: MockTemplate.id,
			parameters: {
				[MockPreviewParameter1.name]: "kept",
				removed_parameter: "gone",
			},
		},
	};

	it("keeps default values a dynamic template may still evaluate", async () => {
		const { user, props } = renderDialog({
			project: mockProjectWithRemovedParameter,
		});

		const kept = await findParameterInput(MockPreviewParameter1.display_name);
		await user.type(kept, "!");
		await user.click(screen.getByRole("button", { name: /Save/ }));

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				attach: [],
				detach: [],
				request: {
					default_template: {
						template_id: MockTemplate.id,
						parameters: {
							[MockPreviewParameter1.name]: "kept!",
							removed_parameter: "gone",
						},
					},
				},
			}),
		);
	});

	it("replaces a saved empty value with the template default", async () => {
		const { user, props } = renderDialog({
			project: {
				...mockProjectWithDefault,
				default_template: {
					template_id: MockTemplate.id,
					parameters: { [MockPreviewParameter1.name]: "" },
				},
			},
		});

		await user.click(
			await screen.findByRole("button", {
				name: `Use template default for ${MockPreviewParameter1.display_name}`,
			}),
		);
		expect(
			screen.getByRole("group", { name: MockPreviewParameter1.display_name }),
		).toHaveFocus();
		await user.click(screen.getByRole("button", { name: /Save/ }));

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				attach: [],
				detach: [],
				request: { default_template: { template_id: MockTemplate.id } },
			}),
		);
	});

	it("sends a save once even when Save is pressed again before the dialog closes", async () => {
		const { user, props } = renderDialog();
		await user.click(await screen.findByRole("button", { name: "Select all" }));
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));

		await user.click(save);
		await waitFor(() => expect(props.onSave).toHaveBeenCalled());
		await user.click(save);
		await user.keyboard("{Enter}");
		await new Promise((resolve) => setTimeout(resolve, 0));

		expect(props.onSave).toHaveBeenCalledTimes(1);
	});

	it("leaves alone an attached template that is missing from the eligible list", async () => {
		const { user, props } = renderDialog({
			project: {
				...MockChatProject,
				templates: [{ ...MockTemplate, id: "deprecated-since-loading" }],
			},
		});

		await user.click(await screen.findByRole("button", { name: "Select all" }));
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		await user.click(save);

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				attach: [MockTemplate.id],
				detach: [],
			}),
		);
	});
});
