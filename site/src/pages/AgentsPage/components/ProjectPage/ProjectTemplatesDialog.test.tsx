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
	const getProjects = vi
		.spyOn(API.experimental, "getChatProjects")
		.mockResolvedValue([props.project ?? MockChatProject]);
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
	const { rerender } = render(
		<QueryClientProvider client={queryClient}>
			<ProjectTemplatesDialog {...allProps} />
		</QueryClientProvider>,
	);
	return {
		user: userEvent.setup(),
		props: allProps,
		rerenderWithProject: (project: ChatProject) => {
			getProjects.mockResolvedValue([project]);
			rerender(
				<QueryClientProvider client={queryClient}>
					<ProjectTemplatesDialog {...allProps} project={project} />
				</QueryClientProvider>,
			);
		},
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

	it("pauses saving until the project refresh succeeds", async () => {
		const getProjects = vi
			.spyOn(API.experimental, "getChatProjects")
			.mockRejectedValueOnce(mockApiError({ message: "Network error." }));
		const { user, props } = renderDialog();

		await user.click(await screen.findByRole("button", { name: "Select all" }));
		await user.click(screen.getByRole("button", { name: /Save/ }));
		expect(props.onSave).not.toHaveBeenCalled();

		await user.click(await screen.findByRole("button", { name: "Retry" }));
		await waitFor(() => expect(getProjects).toHaveBeenCalledTimes(2));
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		await user.click(save);
		await waitFor(() => expect(props.onSave).toHaveBeenCalled());
	});

	it("pauses saving while the project refresh is in flight", async () => {
		vi.spyOn(API.experimental, "getChatProjects").mockReturnValueOnce(
			new Promise(() => {}),
		);
		const { user, props } = renderDialog();

		await user.click(await screen.findByRole("button", { name: "Select all" }));
		await user.click(screen.getByRole("button", { name: /Save/ }));

		expect(props.onSave).not.toHaveBeenCalled();
	});

	it("keeps a saved empty parameter value when the user keeps their draft", async () => {
		const { user, props, rerenderWithProject } = renderDialog({
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
		rerenderWithProject({
			...mockProjectWithDefault,
			default_template: {
				template_id: MockTemplate.id,
				parameters: { [MockPreviewParameter2.name]: "other" },
			},
		});
		await user.click(screen.getByRole("button", { name: "Keep my draft" }));
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

	it("does not save over templates changed since the last refresh", async () => {
		const { user, props } = renderDialog();
		const getProjects = vi.mocked(API.experimental.getChatProjects);
		await user.click(await screen.findByRole("button", { name: "Select all" }));
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		const callsBeforeSave = getProjects.mock.calls.length;
		getProjects.mockResolvedValue([mockProjectWithDefault]);

		await user.click(save);

		await waitFor(() =>
			expect(getProjects).toHaveBeenCalledTimes(callsBeforeSave + 1),
		);
		expect(props.onSave).not.toHaveBeenCalled();
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

	it("saves nothing when the draft changes during the check", async () => {
		const { user, props } = renderDialog();
		const getProjects = vi.mocked(API.experimental.getChatProjects);
		await user.click(await screen.findByRole("button", { name: "Select all" }));
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		let finishCheck: (projects: ChatProject[]) => void = () => {};
		getProjects.mockReturnValueOnce(
			new Promise((resolve) => {
				finishCheck = resolve;
			}),
		);

		await user.click(save);
		await user.click(screen.getByRole("button", { name: "Clear all" }));
		finishCheck([MockChatProject]);

		await waitFor(() => expect(getProjects).toHaveBeenCalled());
		await new Promise((resolve) => setTimeout(resolve, 0));
		expect(props.onSave).not.toHaveBeenCalled();
	});

	it("cancels the save when the dialog closes during the check", async () => {
		const { user, props } = renderDialog();
		const getProjects = vi.mocked(API.experimental.getChatProjects);
		await user.click(await screen.findByRole("button", { name: "Select all" }));
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		let finishCheck: (projects: ChatProject[]) => void = () => {};
		getProjects.mockReturnValueOnce(
			new Promise((resolve) => {
				finishCheck = resolve;
			}),
		);

		await user.click(save);
		await user.click(screen.getByRole("button", { name: "Cancel" }));
		expect(props.onOpenChange).toHaveBeenCalledWith(false);
		finishCheck([MockChatProject]);

		await waitFor(() => expect(getProjects).toHaveBeenCalled());
		await new Promise((resolve) => setTimeout(resolve, 0));
		expect(props.onSave).not.toHaveBeenCalled();
	});

	it("moves focus to the conflict warning when the check finds a newer save", async () => {
		const { user, rerenderWithProject } = renderDialog();
		const getProjects = vi.mocked(API.experimental.getChatProjects);
		await user.click(await screen.findByRole("button", { name: "Select all" }));
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		const newer = mockProjectWithDefault;
		getProjects.mockResolvedValue([newer]);

		await user.click(save);
		rerenderWithProject(newer);

		// The warning, not Load latest, so a repeated Enter keeps the draft.
		await waitFor(() =>
			expect(document.activeElement).toHaveTextContent(
				/Someone else changed these templates/,
			),
		);
		expect(
			screen.getByRole("button", { name: "Load latest" }),
		).not.toHaveFocus();
	});

	it("moves focus to Retry when the check fails", async () => {
		const { user } = renderDialog();
		const getProjects = vi.mocked(API.experimental.getChatProjects);
		await user.click(await screen.findByRole("button", { name: "Select all" }));
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		getProjects.mockRejectedValueOnce(
			mockApiError({ message: "Network error." }),
		);

		await user.click(save);

		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Retry" })).toHaveFocus(),
		);
	});

	it("treats an edited and restored parameter as untouched", async () => {
		const { user, props, rerenderWithProject } = renderDialog({
			project: {
				...mockProjectWithDefault,
				default_template: {
					template_id: MockTemplate.id,
					parameters: {
						[MockPreviewParameter1.name]: "a",
						[MockPreviewParameter2.name]: "b",
					},
				},
			},
		});
		const first = await findParameterInput(MockPreviewParameter1.display_name);
		await user.clear(first);
		await user.type(first, "a");

		// An untouched draft follows the newer save instead of warning.
		rerenderWithProject({
			...mockProjectWithDefault,
			default_template: {
				template_id: MockTemplate.id,
				parameters: { [MockPreviewParameter1.name]: "z" },
			},
		});
		await user.type(
			await findParameterInput(MockPreviewParameter1.display_name),
			"!",
		);
		await user.click(screen.getByRole("button", { name: /Save/ }));

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				attach: [],
				detach: [],
				request: {
					default_template: {
						template_id: MockTemplate.id,
						parameters: { [MockPreviewParameter1.name]: "z!" },
					},
				},
			}),
		);
	});

	it("retries loading the eligible templates and then allows saving", async () => {
		const getTemplates = vi
			.spyOn(API, "getTemplates")
			.mockRejectedValueOnce(mockApiError({ message: "Network error." }));
		const { user, props } = renderDialog();

		await user.click(await screen.findByRole("button", { name: "Retry" }));
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
		// The failed load, Retry, and the pre-save check.
		expect(getTemplates).toHaveBeenCalledTimes(3);
	});

	it("saves nothing when an eligible template changes during the check", async () => {
		const { user, props } = renderDialog();
		await user.click(await screen.findByRole("button", { name: "Select all" }));
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		vi.mocked(API.getTemplates).mockResolvedValue([
			{ ...MockTemplate, active_version_id: "new-active-version" },
		]);

		await user.click(save);

		await waitFor(() => expect(API.getTemplates).toHaveBeenCalledTimes(2));
		await new Promise((resolve) => setTimeout(resolve, 0));
		expect(props.onSave).not.toHaveBeenCalled();
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

	it("saves when only a template outside the draft changes during the check", async () => {
		const { user, props } = renderDialog();
		await user.click(await screen.findByRole("button", { name: "Select all" }));
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		vi.mocked(API.getTemplates).mockResolvedValue([
			MockTemplate,
			{ ...MockTemplate, id: "unrelated-template" },
		]);

		await user.click(save);

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				attach: [MockTemplate.id],
				detach: [],
			}),
		);
	});
});
