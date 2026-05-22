import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { API } from "#/api/api";
import type { DynamicParametersResponse } from "#/api/typesGenerated";
import {
	MockDropdownParameter,
	MockDynamicParametersResponse,
	MockDynamicParametersResponseWithError,
	MockPermissions,
	MockPreviewParameter,
	MockSliderParameter,
	MockTemplate,
	MockTemplateVersionExternalAuthGithub,
	MockTemplateVersionExternalAuthGithubAuthenticated,
	MockUserOwner,
	MockValidationParameter,
	MockWorkspace,
} from "#/testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import { mockDynamicParameterWebSocket } from "#/testHelpers/websockets";
import CreateWorkspacePage from "./CreateWorkspacePage";

describe("CreateWorkspacePage", () => {
	const renderCreateWorkspacePage = (
		route = `/templates/${MockTemplate.name}/workspace`,
	) => {
		return renderWithAuth(<CreateWorkspacePage />, {
			route,
			path: "/templates/:template/workspace",
			extraRoutes: [
				{
					path: "/:username/:workspace",
					element: <div>Workspace Page</div>,
				},
			],
		});
	};

	beforeEach(() => {
		vi.clearAllMocks();

		vi.spyOn(API, "getTemplate").mockResolvedValue(MockTemplate);
		vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([]);
		vi.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([]);
		vi.spyOn(API, "createWorkspace").mockResolvedValue(MockWorkspace);
		vi.spyOn(API, "checkAuthorization").mockResolvedValue(MockPermissions);
		mockDynamicParameterWebSocket(MockDynamicParametersResponse);
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
	});

	describe("WebSocket Integration", () => {
		it("establishes WebSocket connection and receives initial parameters", async () => {
			renderCreateWorkspacePage();

			await waitForLoaderToBeRemoved();

			expect(API.templateVersionDynamicParameters).toHaveBeenCalledWith(
				MockTemplate.active_version_id,
				MockUserOwner.id,
				expect.objectContaining({
					onMessage: expect.any(Function),
					onError: expect.any(Function),
					onClose: expect.any(Function),
				}),
			);

			await waitFor(() => {
				expect(screen.getByText(/instance type/i)).toBeInTheDocument();
				expect(screen.getByText("CPU Count")).toBeInTheDocument();
				expect(screen.getByText("Enable Monitoring")).toBeInTheDocument();
				expect(screen.getByText("Tags")).toBeInTheDocument();
			});
		});

		it("sends parameter updates via WebSocket when form values change", async () => {
			const [mockWebSocket] = mockDynamicParameterWebSocket(
				MockDynamicParametersResponse,
			);

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();

			const instanceTypeField = screen.getByTestId(
				"parameter-field-instance_type",
			);
			const instanceTypeSelect =
				within(instanceTypeField).getByRole("combobox");
			expect(instanceTypeSelect).toBeInTheDocument();

			vi.useFakeTimers({ shouldAdvanceTime: true });

			await userEvent.click(instanceTypeSelect);

			const mediumOption = await screen.findByRole("option", {
				name: /t3\.medium/i,
			});

			await userEvent.click(mediumOption);

			await act(async () => {
				await vi.runAllTimersAsync();
			});

			expect(mockWebSocket.send).toHaveBeenCalledWith(
				expect.stringContaining('"instance_type":"t3.medium"'),
			);

			vi.useRealTimers();
		});

		it("handles WebSocket error gracefully", async () => {
			const [, mockPublisher] = mockDynamicParameterWebSocket([]);

			renderCreateWorkspacePage();

			await waitFor(() => {
				expect(API.templateVersionDynamicParameters).toHaveBeenCalled();
			});

			await act(async () => {
				mockPublisher.publishError(new Event("Connection failed"));
			});

			await waitFor(() => {
				const alert = screen.getByRole("alert");
				expect(
					within(alert).getByRole("heading", {
						name: /connection for dynamic parameters failed/i,
					}),
				).toBeInTheDocument();
			});
		});

		it("handles WebSocket close event", async () => {
			const [, mockPublisher] = mockDynamicParameterWebSocket([]);

			renderCreateWorkspacePage();

			await waitFor(() => {
				expect(API.templateVersionDynamicParameters).toHaveBeenCalled();
			});

			await act(async () => {
				mockPublisher.publishClose(new Event("close") as CloseEvent);
			});

			await waitFor(() => {
				const alert = screen.getByRole("alert");
				expect(
					within(alert).getByRole("heading", {
						name: /websocket connection.*unexpectedly closed/i,
					}),
				).toBeInTheDocument();
			});
		});

		it("only parameters from latest response are displayed", async () => {
			const [, mockPublisher] = mockDynamicParameterWebSocket([
				MockDropdownParameter,
			]);

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			const response1: DynamicParametersResponse = {
				id: 1,
				parameters: [MockDropdownParameter],
				diagnostics: [],
			};
			const response2: DynamicParametersResponse = {
				id: 4,
				parameters: [MockSliderParameter],
				diagnostics: [],
			};

			await act(async () => {
				mockPublisher.publishMessage(
					new MessageEvent("message", { data: JSON.stringify(response1) }),
				);

				mockPublisher.publishMessage(
					new MessageEvent("message", { data: JSON.stringify(response2) }),
				);
			});

			await waitFor(() => {
				expect(screen.queryByText("CPU Count")).toBeInTheDocument();
				expect(screen.queryByText("Instance Type")).not.toBeInTheDocument();
			});
		});

		it("does not clobber user values", async () => {
			const [, mockPublisher] = mockDynamicParameterWebSocket([
				MockPreviewParameter,
			]);

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			const form = screen.getByTestId("form");
			const input = await within(form).findByRole("textbox", {
				name: /parameter 1/i,
			});
			await userEvent.clear(input);
			await userEvent.type(input, "hi there hello");

			await waitFor(() => {
				expect(
					within(form).getByDisplayValue("hi there hello"),
				).toBeInTheDocument();
			});

			// Simulate a stale response.
			await act(async () => {
				mockPublisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: 1,
							parameters: [MockPreviewParameter, MockValidationParameter],
						}),
					}),
				);
			});

			// Should have the new field, but keep the existing user-filled values.
			await waitFor(() => {
				expect(within(form).getByDisplayValue("50")).toBeInTheDocument();
				expect(
					within(form).getByDisplayValue("hi there hello"),
				).toBeInTheDocument();
			});
		});

		it("does not clobber auto-filled values", async () => {
			const [, mockPublisher] = mockDynamicParameterWebSocket([
				MockPreviewParameter,
				MockSliderParameter,
			]);

			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?param.cpu_count=44&param.parameter1=auto`,
			);
			await waitForLoaderToBeRemoved();

			// Simulate a stale response.
			await act(async () => {
				mockPublisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: 2,
							parameters: [
								MockPreviewParameter,
								MockSliderParameter,
								MockValidationParameter,
							],
						}),
					}),
				);
			});

			// Should have the new field, but keep the existing auto-filled values.
			const form = screen.getByTestId("form");
			await waitFor(() => {
				expect(within(form).getByDisplayValue("50")).toBeInTheDocument();
				expect(within(form).getByDisplayValue("44")).toBeInTheDocument();
				expect(within(form).getByDisplayValue("auto")).toBeInTheDocument();
			});
		});
	});

	describe("Dynamic Parameter Types", () => {
		it("displays parameter validation errors", async () => {
			mockDynamicParameterWebSocket(MockDynamicParametersResponseWithError);

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Validation failed")).toBeInTheDocument();
				expect(
					screen.getByText(
						"The selected instance type is not available in this region",
					),
				).toBeInTheDocument();
			});
		});

		it("displays parameter validation errors for min/max constraints", async () => {
			const mockResponseInitial: DynamicParametersResponse = {
				id: 1,
				parameters: [MockValidationParameter],
				diagnostics: [],
			};

			const mockResponseWithError: DynamicParametersResponse = {
				id: 2,
				parameters: [
					{
						...MockValidationParameter,
						value: { value: "200", valid: false },
						diagnostics: [
							{
								severity: "error",
								summary:
									"Invalid parameter value according to 'validation' block",
								detail: "value 200 is more than the maximum 100",
								extra: {
									code: "",
								},
							},
						],
					},
				],
				diagnostics: [],
			};

			const [mockWebSocket, publisher] =
				mockDynamicParameterWebSocket(mockResponseInitial);
			const originalSend = mockWebSocket.send;
			mockWebSocket.send = vi.fn((data) => {
				originalSend.call(mockWebSocket, data);

				if (typeof data === "string" && data.includes('"200"')) {
					publisher.publishMessage(
						new MessageEvent("message", {
							data: JSON.stringify(mockResponseWithError),
						}),
					);
				}
			});

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Invalid Parameter")).toBeInTheDocument();
			});

			const numberInput = screen.getByDisplayValue("50");
			expect(numberInput).toBeInTheDocument();

			await userEvent.clear(numberInput);
			await userEvent.type(numberInput, "200");

			await waitFor(() => {
				expect(screen.getByDisplayValue("200")).toBeInTheDocument();
			});

			await waitFor(() => {
				expect(
					screen.getByText(
						"Invalid parameter value according to 'validation' block",
					),
				).toBeInTheDocument();
			});

			await waitFor(() => {
				expect(
					screen.getByText("value 200 is more than the maximum 100"),
				).toBeInTheDocument();
			});

			const errorElement = screen.getByText(
				"value 200 is more than the maximum 100",
			);
			expect(errorElement.closest("div")).toHaveClass(
				"text-content-destructive",
			);
		});
	});

	describe("External Authentication", () => {
		it("displays external auth providers", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithub,
			]);

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("GitHub")).toBeInTheDocument();
				expect(
					screen.getByRole("button", { name: /login with github/i }),
				).toBeInTheDocument();
			});
		});

		it("shows authenticated state for connected providers", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithubAuthenticated,
			]);

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("GitHub")).toBeInTheDocument();
				expect(screen.getByText(/authenticated/i)).toBeInTheDocument();
			});
		});

		it("prevents auto-creation when required external auth is missing", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithub,
			]);

			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?mode=auto&version=${MockTemplate.id}`,
			);
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(
					screen.getByText(
						/external authentication provider that is not connected/i,
					),
				).toBeInTheDocument();
				expect(
					screen.getByText(/auto-creation has been disabled/i),
				).toBeInTheDocument();
			});
		});
	});

	describe("Auto-creation Mode", () => {
		it("falls back to form mode when auto-creation fails", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithubAuthenticated,
			]);
			vi.spyOn(API, "createWorkspace").mockRejectedValue(
				new Error("Auto-creation failed"),
			);

			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?mode=auto`,
			);

			// Consent dialog appears for mode=auto — confirm to proceed.
			const confirmButton = await screen.findByRole("button", {
				name: /confirm and create/i,
			});
			await userEvent.click(confirmButton);

			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();

			await waitFor(() => {
				expect(screen.getByText("Create workspace")).toBeInTheDocument();
				expect(
					screen.getByRole("button", { name: /create workspace/i }),
				).toBeInTheDocument();
			});
		});
	});

	describe("Form Submission", () => {
		it("creates workspace with correct parameters", async () => {
			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();

			const nameInput = screen.getByRole("textbox", {
				name: /workspace name/i,
			});
			await userEvent.clear(nameInput);
			await userEvent.type(nameInput, "my-test-workspace");

			const createButton = screen.getByRole("button", {
				name: /create workspace/i,
			});
			await userEvent.click(createButton);

			await waitFor(() => {
				expect(API.createWorkspace).toHaveBeenCalledWith(
					"test-user",
					expect.objectContaining({
						name: "my-test-workspace",
						template_version_id: MockTemplate.active_version_id,
						template_id: undefined,
						rich_parameter_values: [
							expect.objectContaining({ name: "instance_type", value: "" }),
							expect.objectContaining({ name: "cpu_count", value: "2" }),
							expect.objectContaining({
								name: "enable_monitoring",
								value: "true",
							}),
							expect.objectContaining({ name: "tags", value: "[]" }),
							expect.objectContaining({ name: "ides", value: "[]" }),
						],
					}),
				);
			});
		});
	});

	describe("URL Parameters", () => {
		it("pre-fills parameters from URL", async () => {
			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?param.instance_type=t3.large&param.cpu_count=4`,
			);
			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();
			expect(screen.getByText("CPU Count")).toBeInTheDocument();
		});

		it("uses custom template version when specified", async () => {
			const customVersionId = "custom-version-123";

			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?version=${customVersionId}`,
			);

			await waitFor(() => {
				expect(API.templateVersionDynamicParameters).toHaveBeenCalledWith(
					customVersionId,
					MockUserOwner.id,
					expect.any(Object),
				);
			});
		});

		it("pre-fills workspace name from URL", async () => {
			const workspaceName = "my-custom-workspace";

			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?name=${workspaceName}`,
			);
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				const nameInput = screen.getByRole("textbox", {
					name: /workspace name/i,
				});
				expect(nameInput).toHaveValue(workspaceName);
			});
		});
	});

	describe("Navigation", () => {
		it("navigates to workspace after successful creation", async () => {
			const { router } = renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			const nameInput = screen.getByRole("textbox", {
				name: /workspace name/i,
			});

			await userEvent.clear(nameInput);
			await userEvent.type(nameInput, "my-test-workspace");

			const createButton = screen.getByRole("button", {
				name: /create workspace/i,
			});
			await userEvent.click(createButton);

			await waitFor(() => {
				expect(router.state.location.pathname).toBe(
					`/@${MockWorkspace.owner_name}/${MockWorkspace.name}`,
				);
			});
		});
	});
});
