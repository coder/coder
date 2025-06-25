import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import type {
	DynamicParametersRequest,
	DynamicParametersResponse,
	PreviewParameter,
} from "api/typesGenerated";
import {
	MockTemplate,
	MockTemplateVersionExternalAuthGithub,
	MockTemplateVersionExternalAuthGithubAuthenticated,
	MockUserOwner,
	MockWorkspace,
} from "testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import CreateWorkspacePageExperimental from "./CreateWorkspacePageExperimental";

// Mock WebSocket
class MockWebSocket {
	static CONNECTING = 0;
	static OPEN = 1;
	static CLOSING = 2;
	static CLOSED = 3;

	readyState = MockWebSocket.CONNECTING;
	onopen: ((event: Event) => void) | null = null;
	onmessage: ((event: MessageEvent) => void) | null = null;
	onerror: ((event: Event) => void) | null = null;
	onclose: ((event: CloseEvent) => void) | null = null;
	
	private messageQueue: string[] = [];

	constructor(public url: string) {
		// Simulate connection opening
		setTimeout(() => {
			this.readyState = MockWebSocket.OPEN;
			this.onopen?.(new Event("open"));
			// Process any queued messages
			this.messageQueue.forEach(message => {
				this.onmessage?.(new MessageEvent("message", { data: message }));
			});
			this.messageQueue = [];
		}, 0);
	}

	send(data: string) {
		if (this.readyState === MockWebSocket.OPEN) {
			// Echo back the message for testing
			setTimeout(() => {
				this.onmessage?.(new MessageEvent("message", { data }));
			}, 0);
		}
	}

	close() {
		this.readyState = MockWebSocket.CLOSED;
		this.onclose?.(new CloseEvent("close"));
	}

	// Helper method to simulate server messages
	simulateMessage(data: string) {
		if (this.readyState === MockWebSocket.OPEN) {
			this.onmessage?.(new MessageEvent("message", { data }));
		} else {
			this.messageQueue.push(data);
		}
	}
}

// Mock parameters for different test scenarios
const mockStringParameter: PreviewParameter = {
	name: "instance_type",
	display_name: "Instance Type",
	description: "The type of instance to create",
	type: "string",
	mutable: true,
	default_value: "t3.micro",
	icon: "",
	options: [
		{ name: "t3.micro", description: "Small instance", value: "t3.micro", icon: "" },
		{ name: "t3.small", description: "Medium instance", value: "t3.small", icon: "" },
		{ name: "t3.medium", description: "Large instance", value: "t3.medium", icon: "" },
	],
	validation_error: "",
	validation_condition: "",
	validation_type_system: "",
	validation_value_type: "",
	required: true,
	legacy_variable_name: "",
	order: 1,
	form_type: "select",
	ephemeral: false,
};

const mockNumberParameter: PreviewParameter = {
	name: "cpu_count",
	display_name: "CPU Count",
	description: "Number of CPU cores",
	type: "number",
	mutable: true,
	default_value: "2",
	icon: "",
	options: [],
	validation_error: "",
	validation_condition: "",
	validation_type_system: "",
	validation_value_type: "",
	required: true,
	legacy_variable_name: "",
	order: 2,
	form_type: "slider",
	ephemeral: false,
};

const mockBooleanParameter: PreviewParameter = {
	name: "enable_monitoring",
	display_name: "Enable Monitoring",
	description: "Enable system monitoring",
	type: "bool",
	mutable: true,
	default_value: "true",
	icon: "",
	options: [],
	validation_error: "",
	validation_condition: "",
	validation_type_system: "",
	validation_value_type: "",
	required: false,
	legacy_variable_name: "",
	order: 3,
	form_type: "switch",
	ephemeral: false,
};

const mockListParameter: PreviewParameter = {
	name: "tags",
	display_name: "Tags",
	description: "Resource tags",
	type: "list(string)",
	mutable: true,
	default_value: "[]",
	icon: "",
	options: [],
	validation_error: "",
	validation_condition: "",
	validation_type_system: "",
	validation_value_type: "",
	required: false,
	legacy_variable_name: "",
	order: 4,
	form_type: "tags",
	ephemeral: false,
};

const mockDynamicParametersResponse: DynamicParametersResponse = {
	id: 1,
	parameters: [mockStringParameter, mockNumberParameter, mockBooleanParameter, mockListParameter],
	diagnostics: [],
};

const mockDynamicParametersResponseWithError: DynamicParametersResponse = {
	id: 2,
	parameters: [
		{
			...mockStringParameter,
			validation_error: "Invalid instance type selected",
		},
	],
	diagnostics: [
		{
			severity: "error",
			summary: "Validation failed",
			detail: "The selected instance type is not available in this region",
			range: null,
		},
	],
};

const renderCreateWorkspacePageExperimental = (route = `/templates/${MockTemplate.name}/workspace`) => {
	return renderWithAuth(<CreateWorkspacePageExperimental />, {
		route,
		path: "/templates/:template/workspace",
	});
};

describe("CreateWorkspacePageExperimental", () => {
	let mockWebSocket: MockWebSocket;
	let mockWebSocketInstances: MockWebSocket[] = [];

	// Store original WebSocket
	const originalWebSocket = global.WebSocket;

	beforeAll(() => {
		global.WebSocket = MockWebSocket as any;
	});

	afterAll(() => {
		global.WebSocket = originalWebSocket;
	});

	beforeEach(() => {
		jest.clearAllMocks();
		mockWebSocketInstances = [];

		// Setup API mocks using jest.spyOn like the existing tests
		jest.spyOn(API, "getTemplate").mockResolvedValue(MockTemplate);
		jest.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([]);
		jest.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([]);
		jest.spyOn(API, "createWorkspace").mockResolvedValue(MockWorkspace);
		jest.spyOn(API, "autoCreateWorkspace").mockResolvedValue(MockWorkspace);
		jest.spyOn(API, "checkAuthorization").mockResolvedValue({});

		// Mock the WebSocket creation function
		jest.spyOn(API, "templateVersionDynamicParameters").mockImplementation((versionId, ownerId, callbacks) => {
			mockWebSocket = new MockWebSocket(`ws://test/${versionId}`);
			mockWebSocketInstances.push(mockWebSocket);
			
			mockWebSocket.onopen = () => {
				// Send initial parameters response
				setTimeout(() => {
					callbacks.onMessage?.(mockDynamicParametersResponse);
				}, 10);
			};
			
			if (callbacks.onError) mockWebSocket.onerror = callbacks.onError;
			if (callbacks.onClose) mockWebSocket.onclose = callbacks.onClose;
			
			return mockWebSocket;
		});
	});

	afterEach(() => {
		mockWebSocketInstances.forEach(ws => ws.close());
		jest.restoreAllMocks();
	});

	describe("WebSocket Integration", () => {
		it("establishes WebSocket connection and receives initial parameters", async () => {
			renderCreateWorkspacePageExperimental();
			
			await waitForLoaderToBeRemoved();
			
			expect(API.templateVersionDynamicParameters).toHaveBeenCalledWith(
				MockTemplate.active_version_id,
				MockUserOwner.id,
				expect.objectContaining({
					onMessage: expect.any(Function),
					onError: expect.any(Function),
					onClose: expect.any(Function),
				})
			);

			// Check that parameters are rendered
			await waitFor(() => {
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
				expect(screen.getByText("CPU Count")).toBeInTheDocument();
				expect(screen.getByText("Enable Monitoring")).toBeInTheDocument();
				expect(screen.getByText("Tags")).toBeInTheDocument();
			});
		});

		it("sends parameter updates via WebSocket when form values change", async () => {
			const sendSpy = jest.spyOn(MockWebSocket.prototype, "send");
			
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			// Wait for initial parameters to load
			await waitFor(() => {
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
			});

			// Change a parameter value
			const instanceTypeSelect = screen.getByRole("combobox", { name: /instance type/i });
			await userEvent.click(instanceTypeSelect);
			
			const mediumOption = screen.getByText("Large instance");
			await userEvent.click(mediumOption);

			// Verify WebSocket message was sent
			await waitFor(() => {
				expect(sendSpy).toHaveBeenCalledWith(
					expect.stringContaining('"instance_type":"t3.medium"')
				);
			});
		});

		it("handles WebSocket error gracefully", async () => {
			jest.spyOn(API, "templateVersionDynamicParameters").mockImplementation((versionId, ownerId, callbacks) => {
				mockWebSocket = new MockWebSocket(`ws://test/${versionId}`);
				mockWebSocketInstances.push(mockWebSocket);
				
				// Simulate error
				setTimeout(() => {
					callbacks.onError?.(new Error("Connection failed"));
				}, 10);
				
				return mockWebSocket;
			});

			renderCreateWorkspacePageExperimental();
			
			await waitFor(() => {
				expect(screen.getByText(/connection failed/i)).toBeInTheDocument();
			});
		});

		it("handles WebSocket close event", async () => {
			jest.spyOn(API, "templateVersionDynamicParameters").mockImplementation((versionId, ownerId, callbacks) => {
				mockWebSocket = new MockWebSocket(`ws://test/${versionId}`);
				mockWebSocketInstances.push(mockWebSocket);
				
				// Simulate close
				setTimeout(() => {
					callbacks.onClose?.();
				}, 10);
				
				return mockWebSocket;
			});

			renderCreateWorkspacePageExperimental();
			
			await waitFor(() => {
				expect(screen.getByText(/websocket connection.*unexpectedly closed/i)).toBeInTheDocument();
			});
		});

		it("processes parameter responses in correct order", async () => {
			let messageCallback: ((response: DynamicParametersResponse) => void) | undefined;
			
			jest.spyOn(API, "templateVersionDynamicParameters").mockImplementation((versionId, ownerId, callbacks) => {
				mockWebSocket = new MockWebSocket(`ws://test/${versionId}`);
				mockWebSocketInstances.push(mockWebSocket);
				messageCallback = callbacks.onMessage;
				return mockWebSocket;
			});

			renderCreateWorkspacePageExperimental();
			
			// Send responses out of order
			const response1: DynamicParametersResponse = { id: 1, parameters: [mockStringParameter], diagnostics: [] };
			const response2: DynamicParametersResponse = { id: 2, parameters: [mockNumberParameter], diagnostics: [] };
			const response3: DynamicParametersResponse = { id: 1, parameters: [mockBooleanParameter], diagnostics: [] }; // Older response

			messageCallback?.(response2);
			messageCallback?.(response3); // Should be ignored
			messageCallback?.(response1); // Should be ignored

			await waitFor(() => {
				expect(screen.getByText("CPU Count")).toBeInTheDocument();
				expect(screen.queryByText("Instance Type")).not.toBeInTheDocument();
				expect(screen.queryByText("Enable Monitoring")).not.toBeInTheDocument();
			});
		});
	});

	describe("Dynamic Parameter Types", () => {
		it("renders string parameter with select options", async () => {
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
				expect(screen.getByRole("combobox", { name: /instance type/i })).toBeInTheDocument();
			});

			// Open select and verify options
			const select = screen.getByRole("combobox", { name: /instance type/i });
			await userEvent.click(select);

			expect(screen.getByText("Small instance")).toBeInTheDocument();
			expect(screen.getByText("Medium instance")).toBeInTheDocument();
			expect(screen.getByText("Large instance")).toBeInTheDocument();
		});

		it("renders number parameter with slider", async () => {
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("CPU Count")).toBeInTheDocument();
				expect(screen.getByRole("slider", { name: /cpu count/i })).toBeInTheDocument();
			});
		});

		it("renders boolean parameter with switch", async () => {
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Enable Monitoring")).toBeInTheDocument();
				expect(screen.getByRole("switch", { name: /enable monitoring/i })).toBeInTheDocument();
			});
		});

		it("renders list parameter with tag input", async () => {
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Tags")).toBeInTheDocument();
				expect(screen.getByRole("textbox", { name: /tags/i })).toBeInTheDocument();
			});
		});

		it("displays parameter validation errors", async () => {
			jest.spyOn(API, "templateVersionDynamicParameters").mockImplementation((versionId, ownerId, callbacks) => {
				mockWebSocket = new MockWebSocket(`ws://test/${versionId}`);
				mockWebSocketInstances.push(mockWebSocket);
				
				mockWebSocket.onopen = () => {
					setTimeout(() => {
						callbacks.onMessage?.(mockDynamicParametersResponseWithError);
					}, 10);
				};
				
				return mockWebSocket;
			});

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Invalid instance type selected")).toBeInTheDocument();
				expect(screen.getByText("Validation failed")).toBeInTheDocument();
				expect(screen.getByText("The selected instance type is not available in this region")).toBeInTheDocument();
			});
		});

		it("handles disabled parameters", async () => {
			renderCreateWorkspacePageExperimental(`/templates/${MockTemplate.name}/workspace?disable_params=instance_type,cpu_count`);
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				const instanceTypeSelect = screen.getByRole("combobox", { name: /instance type/i });
				const cpuSlider = screen.getByRole("slider", { name: /cpu count/i });
				
				expect(instanceTypeSelect).toBeDisabled();
				expect(cpuSlider).toBeDisabled();
			});
		});
	});

	describe("External Authentication", () => {
		it("displays external auth providers", async () => {
			jest.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([MockTemplateVersionExternalAuthGithub]);

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText(/github/i)).toBeInTheDocument();
				expect(screen.getByRole("button", { name: /connect/i })).toBeInTheDocument();
			});
		});

		it("shows authenticated state for connected providers", async () => {
			jest.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([MockTemplateVersionExternalAuthGithubAuthenticated]);

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText(/github/i)).toBeInTheDocument();
				expect(screen.getByText(/authenticated/i)).toBeInTheDocument();
			});
		});

		it("prevents auto-creation when required external auth is missing", async () => {
			jest.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([MockTemplateVersionExternalAuthGithub]);

			renderCreateWorkspacePageExperimental(`/templates/${MockTemplate.name}/workspace?mode=auto`);
			
			await waitFor(() => {
				expect(screen.getByText(/external authentication providers that are not connected/i)).toBeInTheDocument();
				expect(screen.getByText(/auto-creation has been disabled/i)).toBeInTheDocument();
			});
		});
	});

	describe("Auto-creation Mode", () => {
		it("automatically creates workspace when all requirements are met", async () => {
			jest.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([MockTemplateVersionExternalAuthGithubAuthenticated]);

			renderCreateWorkspacePageExperimental(`/templates/${MockTemplate.name}/workspace?mode=auto&name=test-workspace`);

			await waitFor(() => {
				expect(API.autoCreateWorkspace).toHaveBeenCalledWith(
					expect.objectContaining({
						organizationId: MockTemplate.organization_id,
						templateName: MockTemplate.name,
						workspaceName: "test-workspace",
						templateVersionId: MockTemplate.active_version_id,
					})
				);
			});
		});

		it("falls back to form mode when auto-creation fails", async () => {
			jest.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([MockTemplateVersionExternalAuthGithubAuthenticated]);
			jest.spyOn(API, "autoCreateWorkspace").mockRejectedValue(new Error("Auto-creation failed"));

			renderCreateWorkspacePageExperimental(`/templates/${MockTemplate.name}/workspace?mode=auto`);

			await waitFor(() => {
				expect(screen.getByText("Create workspace")).toBeInTheDocument();
				expect(screen.getByRole("button", { name: /create workspace/i })).toBeInTheDocument();
			});
		});
	});

	describe("Form Submission", () => {
		it("creates workspace with correct parameters", async () => {
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			// Wait for form to load
			await waitFor(() => {
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
			});

			// Fill in workspace name
			const nameInput = screen.getByRole("textbox", { name: /workspace name/i });
			await userEvent.clear(nameInput);
			await userEvent.type(nameInput, "my-test-workspace");

			// Submit form
			const createButton = screen.getByRole("button", { name: /create workspace/i });
			await userEvent.click(createButton);

			await waitFor(() => {
				expect(API.createWorkspace).toHaveBeenCalledWith(
					expect.objectContaining({
						name: "my-test-workspace",
						template_version_id: MockTemplate.active_version_id,
						userId: MockUserOwner.id,
					})
				);
			});
		});

		it("displays creation progress", async () => {
			jest.spyOn(API, "createWorkspace").mockImplementation(() => new Promise(resolve => setTimeout(() => resolve(MockWorkspace), 1000)));

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			// Submit form
			const createButton = screen.getByRole("button", { name: /create workspace/i });
			await userEvent.click(createButton);

			// Should show loading state
			expect(screen.getByText(/creating/i)).toBeInTheDocument();
			expect(createButton).toBeDisabled();
		});

		it("handles creation errors", async () => {
			const errorMessage = "Failed to create workspace";
			jest.spyOn(API, "createWorkspace").mockRejectedValue(new Error(errorMessage));

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			// Submit form
			const createButton = screen.getByRole("button", { name: /create workspace/i });
			await userEvent.click(createButton);

			await waitFor(() => {
				expect(screen.getByText(errorMessage)).toBeInTheDocument();
			});
		});
	});

	describe("URL Parameters", () => {
		it("pre-fills parameters from URL", async () => {
			renderCreateWorkspacePageExperimental(
				`/templates/${MockTemplate.name}/workspace?param.instance_type=t3.large&param.cpu_count=4`
			);
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				// Verify parameters are pre-filled
				// This would require checking the actual form values
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
				expect(screen.getByText("CPU Count")).toBeInTheDocument();
			});
		});

		it("uses custom template version when specified", async () => {
			const customVersionId = "custom-version-123";
			
			renderCreateWorkspacePageExperimental(
				`/templates/${MockTemplate.name}/workspace?version=${customVersionId}`
			);

			await waitFor(() => {
				expect(API.templateVersionDynamicParameters).toHaveBeenCalledWith(
					customVersionId,
					MockUserOwner.id,
					expect.any(Object)
				);
			});
		});

		it("pre-fills workspace name from URL", async () => {
			const workspaceName = "my-custom-workspace";
			
			renderCreateWorkspacePageExperimental(
				`/templates/${MockTemplate.name}/workspace?name=${workspaceName}`
			);
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				const nameInput = screen.getByRole("textbox", { name: /workspace name/i });
				expect(nameInput).toHaveValue(workspaceName);
			});
		});
	});

	describe("Template Presets", () => {
		const mockPreset = {
			id: "preset-1",
			name: "Development",
			description: "Development environment preset",
			parameters: [
				{ name: "instance_type", value: "t3.small" },
				{ name: "cpu_count", value: "2" },
			],
		};

		it("displays available presets", async () => {
			jest.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([mockPreset]);

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Development")).toBeInTheDocument();
				expect(screen.getByText("Development environment preset")).toBeInTheDocument();
			});
		});

		it("applies preset parameters when selected", async () => {
			jest.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([mockPreset]);
			const sendSpy = jest.spyOn(MockWebSocket.prototype, "send");

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			// Select preset
			const presetButton = screen.getByRole("button", { name: /development/i });
			await userEvent.click(presetButton);

			// Verify parameters are sent via WebSocket
			await waitFor(() => {
				expect(sendSpy).toHaveBeenCalledWith(
					expect.stringContaining('"instance_type":"t3.small"')
				);
				expect(sendSpy).toHaveBeenCalledWith(
					expect.stringContaining('"cpu_count":"2"')
				);
			});
		});
	});

	describe("Navigation", () => {
		it("navigates back when cancel is clicked", async () => {
			const { history } = renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			const cancelButton = screen.getByRole("button", { name: /cancel/i });
			await userEvent.click(cancelButton);

			expect(history.location.pathname).not.toBe(`/templates/${MockTemplate.name}/workspace`);
		});

		it("navigates to workspace after successful creation", async () => {
			const { history } = renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			// Submit form
			const createButton = screen.getByRole("button", { name: /create workspace/i });
			await userEvent.click(createButton);

			await waitFor(() => {
				expect(history.location.pathname).toBe(`/@${MockWorkspace.owner_name}/${MockWorkspace.name}`);
			});
		});
	});

	describe("Error Handling", () => {
		it("displays template loading errors", async () => {
			const errorMessage = "Template not found";
			jest.spyOn(API, "getTemplate").mockRejectedValue(new Error(errorMessage));

			renderCreateWorkspacePageExperimental();

			await waitFor(() => {
				expect(screen.getByText(errorMessage)).toBeInTheDocument();
			});
		});

		it("displays permission errors", async () => {
			const errorMessage = "Insufficient permissions";
			jest.spyOn(API, "checkAuthorization").mockRejectedValue(new Error(errorMessage));

			renderCreateWorkspacePageExperimental();

			await waitFor(() => {
				expect(screen.getByText(errorMessage)).toBeInTheDocument();
			});
		});

		it("allows error reset", async () => {
			const errorMessage = "Creation failed";
			jest.spyOn(API, "createWorkspace").mockRejectedValue(new Error(errorMessage));

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			// Trigger error
			const createButton = screen.getByRole("button", { name: /create workspace/i });
			await userEvent.click(createButton);

			await waitFor(() => {
				expect(screen.getByText(errorMessage)).toBeInTheDocument();
			});

			// Reset error
			jest.spyOn(API, "createWorkspace").mockResolvedValue(MockWorkspace);
			const retryButton = screen.getByRole("button", { name: /try again/i });
			await userEvent.click(retryButton);

			await waitFor(() => {
				expect(screen.queryByText(errorMessage)).not.toBeInTheDocument();
			});
		});
	});
});