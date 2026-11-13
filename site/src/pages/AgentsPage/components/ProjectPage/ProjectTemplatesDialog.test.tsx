import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProject, Template } from "#/api/typesGenerated";
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

const renderDialog = (
	props: Partial<DialogProps> = {},
	template: Template = MockTemplate,
) => {
	vi.spyOn(API, "getTemplates").mockResolvedValue([template]);
	const getProjects = vi
		.spyOn(API.experimental, "getChatProjects")
		.mockResolvedValue([props.project ?? MockChatProject]);
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
		await user.click(screen.getByRole("button", { name: /Save/ }));

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				template_ids: [MockTemplate.id],
				default_template_id: MockTemplate.id,
				default_parameters: {
					[MockTemplateVersionParameter1.name]: "",
					[MockTemplateVersionParameter2.name]: "new",
				},
			}),
		);
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

		const save = await screen.findByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		await user.click(save);

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				template_ids: [MockTemplate.id],
				default_template_id: MockTemplate.id,
			}),
		);
	});

	const mockProjectWithRemovedParameter: ChatProject = {
		...MockChatProject,
		templates: {
			template_ids: [MockTemplate.id],
			default_template_id: MockTemplate.id,
			default_parameters: {
				[MockTemplateVersionParameter1.name]: "kept",
				removed_parameter: "gone",
			},
		},
	};

	it("drops default values for parameters a classic template no longer has", async () => {
		const { user, props } = renderDialog(
			{ project: mockProjectWithRemovedParameter },
			{ ...MockTemplate, use_classic_parameter_flow: true },
		);

		const save = await screen.findByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		await user.click(save);

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				template_ids: [MockTemplate.id],
				default_template_id: MockTemplate.id,
				default_parameters: { [MockTemplateVersionParameter1.name]: "kept" },
			}),
		);
	});

	it("keeps default values a dynamic template may still evaluate", async () => {
		const { user, props } = renderDialog({
			project: mockProjectWithRemovedParameter,
		});

		const kept = await screen.findByRole("textbox", {
			name: MockTemplateVersionParameter1.name,
		});
		await user.type(kept, "!");
		await user.click(screen.getByRole("button", { name: /Save/ }));

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				template_ids: [MockTemplate.id],
				default_template_id: MockTemplate.id,
				default_parameters: {
					[MockTemplateVersionParameter1.name]: "kept!",
					removed_parameter: "gone",
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
		const templates = {
			template_ids: [MockTemplate.id],
			default_template_id: MockTemplate.id,
		};
		const { user, props, rerenderWithProject } = renderDialog({
			project: {
				...MockChatProject,
				templates: {
					...templates,
					default_parameters: {
						[MockTemplateVersionParameter1.name]: "",
						[MockTemplateVersionParameter2.name]: "old",
					},
				},
			},
		});

		const second = await screen.findByRole("textbox", {
			name: MockTemplateVersionParameter2.name,
		});
		await user.clear(second);
		await user.type(second, "new");
		rerenderWithProject({
			...MockChatProject,
			templates: {
				...templates,
				default_parameters: { [MockTemplateVersionParameter2.name]: "other" },
			},
		});
		await user.click(screen.getByRole("button", { name: "Keep my draft" }));
		await user.click(screen.getByRole("button", { name: /Save/ }));

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				...templates,
				default_parameters: {
					[MockTemplateVersionParameter1.name]: "",
					[MockTemplateVersionParameter2.name]: "new",
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
		getProjects.mockResolvedValue([
			{
				...MockChatProject,
				templates: {
					template_ids: [MockTemplate.id],
					default_template_id: MockTemplate.id,
				},
			},
		]);

		await user.click(save);

		await waitFor(() =>
			expect(getProjects).toHaveBeenCalledTimes(callsBeforeSave + 1),
		);
		expect(props.onSave).not.toHaveBeenCalled();
	});

	it("replaces a saved empty value with the template default", async () => {
		const templates = {
			template_ids: [MockTemplate.id],
			default_template_id: MockTemplate.id,
		};
		const { user, props } = renderDialog({
			project: {
				...MockChatProject,
				templates: {
					...templates,
					default_parameters: { [MockTemplateVersionParameter1.name]: "" },
				},
			},
		});

		await user.click(
			await screen.findByRole("button", { name: "Use template default" }),
		);
		expect(
			screen.getByRole("textbox", { name: MockTemplateVersionParameter1.name }),
		).toHaveFocus();
		await user.click(screen.getByRole("button", { name: /Save/ }));

		await waitFor(() => expect(props.onSave).toHaveBeenCalledWith(templates));
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
		const newer = {
			...MockChatProject,
			templates: {
				template_ids: [MockTemplate.id],
				default_template_id: MockTemplate.id,
			},
		};
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
		const templates = {
			template_ids: [MockTemplate.id],
			default_template_id: MockTemplate.id,
			default_parameters: {
				[MockTemplateVersionParameter1.name]: "a",
				[MockTemplateVersionParameter2.name]: "b",
			},
		};
		const { user, props, rerenderWithProject } = renderDialog({
			project: { ...MockChatProject, templates },
		});
		const first = await screen.findByRole("textbox", {
			name: MockTemplateVersionParameter1.name,
		});
		await user.clear(first);
		await user.type(first, "a");

		// An untouched draft follows the newer save instead of warning.
		const newer = {
			...templates,
			default_parameters: { [MockTemplateVersionParameter1.name]: "z" },
		};
		rerenderWithProject({ ...MockChatProject, templates: newer });
		await user.type(
			screen.getByRole("textbox", { name: MockTemplateVersionParameter1.name }),
			"!",
		);
		await user.click(screen.getByRole("button", { name: /Save/ }));

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				...newer,
				default_parameters: { [MockTemplateVersionParameter1.name]: "z!" },
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
				template_ids: [MockTemplate.id],
			}),
		);
		// The failed load, Retry, and the pre-save check.
		expect(getTemplates).toHaveBeenCalledTimes(3);
	});

	it("pauses saving until the default preset list loads", async () => {
		const getPresets = vi
			.spyOn(API, "getTemplateVersionPresets")
			.mockRejectedValueOnce(mockApiError({ message: "Network error." }))
			.mockResolvedValue([]);
		const { user, props } = renderDialog({
			project: {
				...MockChatProject,
				templates: {
					template_ids: [MockTemplate.id],
					default_template_id: MockTemplate.id,
					default_preset_id: "preset-id",
				},
			},
		});

		await user.click(await screen.findByRole("button", { name: "Retry" }));
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		await user.click(save);

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				template_ids: [MockTemplate.id],
				default_template_id: MockTemplate.id,
			}),
		);
		expect(getPresets).toHaveBeenCalledTimes(2);
	});

	it("ignores a failed preset list after switching to parameter values", async () => {
		vi.spyOn(API, "getTemplateVersionPresets").mockRejectedValue(
			mockApiError({ message: "Network error." }),
		);
		const { user, props } = renderDialog({
			project: {
				...MockChatProject,
				templates: {
					template_ids: [MockTemplate.id],
					default_template_id: MockTemplate.id,
					default_preset_id: "preset-id",
				},
			},
		});

		await screen.findByRole("button", { name: "Retry" });
		await user.click(screen.getByRole("radio", { name: "Parameter values" }));
		await user.type(
			await screen.findByRole("textbox", {
				name: MockTemplateVersionParameter1.name,
			}),
			"x",
		);
		const save = screen.getByRole("button", { name: /Save/ });
		await waitFor(() => expect(save).toHaveAttribute("aria-disabled", "false"));
		await user.click(save);

		await waitFor(() =>
			expect(props.onSave).toHaveBeenCalledWith({
				template_ids: [MockTemplate.id],
				default_template_id: MockTemplate.id,
				default_parameters: { [MockTemplateVersionParameter1.name]: "x" },
			}),
		);
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
				template_ids: [MockTemplate.id],
			}),
		);
	});
});
