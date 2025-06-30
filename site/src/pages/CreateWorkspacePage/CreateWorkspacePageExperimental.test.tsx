import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import type {
	DynamicParametersResponse,
	PreviewParameter,
} from "api/typesGenerated";
import {
	MockTemplate,
	MockUserOwner,
	MockWorkspace,
} from "testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import CreateWorkspacePageExperimental from "./CreateWorkspacePageExperimental";

beforeAll(() => {
	if (!Element.prototype.hasPointerCapture) {
		Element.prototype.hasPointerCapture = () => false;
	}
	if (!Element.prototype.setPointerCapture) {
		Element.prototype.setPointerCapture = () => {};
	}
	if (!Element.prototype.releasePointerCapture) {
		Element.prototype.releasePointerCapture = () => {};
	}
});

type MockPublisher = Readonly<{
	publishMessage: (event: MessageEvent<string>) => void;
	publishError: (event: ErrorEvent) => void;
	publishClose: (event: CloseEvent) => void;
	publishOpen: (event: Event) => void;
}>;

type WebSocketEventMap = {
	message: MessageEvent<string>;
	error: ErrorEvent;
	close: CloseEvent;
	open: Event;
};

type MockWebSocket = Omit<WebSocket, "readyState"> & {
	readyState: number;
};

function createMockWebSocket(
	url: string,
	protocols?: string | string[],
): readonly [WebSocket, MockPublisher] {
	type CallbackStore = {
		[K in keyof WebSocketEventMap]: ((event: WebSocketEventMap[K]) => void)[];
	};

	let activeProtocol: string;
	if (Array.isArray(protocols)) {
		activeProtocol = protocols[0] ?? "";
	} else if (typeof protocols === "string") {
		activeProtocol = protocols;
	} else {
		activeProtocol = "";
	}

	let closed = false;
	const store: CallbackStore = {
		message: [],
		error: [],
		close: [],
		open: [],
	};

	const mockSocket: MockWebSocket = {
		CONNECTING: 0,
		OPEN: 1,
		CLOSING: 2,
		CLOSED: 3,

		url,
		protocol: activeProtocol,
		readyState: 1,
		binaryType: "blob",
		bufferedAmount: 0,
		extensions: "",
		onclose: null,
		onerror: null,
		onmessage: null,
		onopen: null,
		send: jest.fn(),
		dispatchEvent: jest.fn(),

		addEventListener: <E extends keyof WebSocketEventMap>(
			eventType: E,
			callback: (event: WebSocketEventMap[E]) => void,
		) => {
			if (closed) {
				return;
			}

			const subscribers = store[eventType];
			const cb = callback as unknown as CallbackStore[E][0];
			if (!subscribers.includes(cb)) {
				subscribers.push(cb);
			}
		},

		removeEventListener: <E extends keyof WebSocketEventMap>(
			eventType: E,
			callback: (event: WebSocketEventMap[E]) => void,
		) => {
			if (closed) {
				return;
			}

			const subscribers = store[eventType];
			const cb = callback as unknown as CallbackStore[E][0];
			if (subscribers.includes(cb)) {
				const updated = store[eventType].filter((c) => c !== cb);
				store[eventType] = updated as unknown as CallbackStore[E];
			}
		},

		close: () => {
			if (!closed) {
				closed = true;
				publisher.publishClose(new CloseEvent("close"));
			}
		},
	};

	const publisher: MockPublisher = {
		publishOpen: (event) => {
			if (closed) {
				return;
			}
			for (const sub of store.open) {
				sub(event);
			}
			mockSocket.onopen?.(event);
		},

		publishError: (event) => {
			if (closed) {
				return;
			}
			for (const sub of store.error) {
				sub(event);
			}
			mockSocket.onerror?.(event);
		},

		publishMessage: (event) => {
			if (closed) {
				return;
			}
			for (const sub of store.message) {
				sub(event);
			}
			mockSocket.onmessage?.(event);
		},

		publishClose: (event) => {
			if (closed) {
				return;
			}
			mockSocket.readyState = 3; // CLOSED
			for (const sub of store.close) {
				sub(event);
			}
			mockSocket.onclose?.(event);
		},
	};

	return [mockSocket as WebSocket, publisher] as const;
}

const mockStringParameter: PreviewParameter = {
	name: "instance_type",
	display_name: "Instance Type",
	description: "The type of instance to create",
	type: "string",
	mutable: true,
	default_value: { value: "t3.micro", valid: true },
	icon: "",
	options: [
		{
			name: "t3.micro",
			description: "Micro instance",
			value: { value: "t3.micro", valid: true },
			icon: "",
		},
		{
			name: "t3.small",
			description: "Small instance",
			value: { value: "t3.small", valid: true },
			icon: "",
		},
		{
			name: "t3.medium",
			description: "Medium instance",
			value: { value: "t3.medium", valid: true },
			icon: "",
		},
	],
	validations: [],
	styling: {
		placeholder: "",
		disabled: false,
		label: "",
	},
	diagnostics: [],
	value: { value: "", valid: true },
	required: true,
	order: 1,
	form_type: "dropdown",
	ephemeral: false,
};

const mockNumberParameter: PreviewParameter = {
	name: "cpu_count",
	display_name: "CPU Count",
	description: "Number of CPU cores",
	type: "number",
	mutable: true,
	default_value: { value: "2", valid: true },
	icon: "",
	options: [],
	validations: [],
	styling: {
		placeholder: "",
		disabled: false,
		label: "",
	},
	diagnostics: [],
	value: { value: "2", valid: true },
	required: true,
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
	default_value: { value: "true", valid: true },
	icon: "",
	options: [],
	validations: [],
	styling: {
		placeholder: "",
		disabled: false,
		label: "",
	},
	diagnostics: [],
	value: { value: "true", valid: true },
	required: false,
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
	default_value: { value: "[]", valid: true },
	icon: "",
	options: [],
	validations: [],
	styling: {
		placeholder: "",
		disabled: false,
		label: "",
	},
	diagnostics: [],
	value: { value: "[]", valid: true },
	required: false,
	order: 4,
	form_type: "tag-select",
	ephemeral: false,
};

const mockDynamicParametersResponse: DynamicParametersResponse = {
	id: 1,
	parameters: [
		mockStringParameter,
		mockNumberParameter,
		mockBooleanParameter,
		mockListParameter,
	],
	diagnostics: [],
};

const mockDynamicParametersResponseWithError: DynamicParametersResponse = {
	id: 2,
	parameters: [mockStringParameter],
	diagnostics: [
		{
			severity: "error",
			summary: "Validation failed",
			detail: "The selected instance type is not available in this region",
			extra: {
				code: "",
			},
		},
	],
};

const renderCreateWorkspacePageExperimental = (
	route = `/templates/${MockTemplate.name}/workspace`,
) => {
	return renderWithAuth(<CreateWorkspacePageExperimental />, {
		route,
		path: "/templates/:template/workspace",
	});
};

describe("CreateWorkspacePageExperimental", () => {
	let mockWebSocket: WebSocket;
	let publisher: MockPublisher;

	beforeEach(() => {
		jest.clearAllMocks();

		// Setup API mocks using jest.spyOn like the existing tests
		jest.spyOn(API, "getTemplate").mockResolvedValue(MockTemplate);
		jest.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([]);
		jest.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([]);
		jest.spyOn(API, "createWorkspace").mockResolvedValue(MockWorkspace);
		jest.spyOn(API, "checkAuthorization").mockResolvedValue({});

		// Mock the WebSocket creation function
		jest
			.spyOn(API, "templateVersionDynamicParameters")
			.mockImplementation((versionId, _ownerId, callbacks) => {
				const [socket, pub] = createMockWebSocket(`ws://test/${versionId}`);
				mockWebSocket = socket;
				publisher = pub;

				socket.addEventListener("message", (event) => {
					callbacks.onMessage(JSON.parse(event.data));
				});
				socket.addEventListener("error", (event) => {
					callbacks.onError((event as ErrorEvent).error);
				});
				socket.addEventListener("close", () => {
					callbacks.onClose();
				});

				setTimeout(() => {
					publisher.publishOpen(new Event("open"));
					publisher.publishMessage(
						new MessageEvent("message", {
							data: JSON.stringify(mockDynamicParametersResponse),
						}),
					);
				}, 10);

				return mockWebSocket;
			});
	});

	afterEach(() => {
		mockWebSocket?.close();
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
				}),
			);

			await waitFor(() => {
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
				expect(screen.getByText("CPU Count")).toBeInTheDocument();
				expect(screen.getByText("Enable Monitoring")).toBeInTheDocument();
				expect(screen.getByText("Tags")).toBeInTheDocument();
			});
		});

		it("sends parameter updates via WebSocket when form values change", async () => {
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
			});

			expect(mockWebSocket.send).toBeDefined();

			const instanceTypeSelect = screen.getByRole("combobox", {
				name: /instance type/i,
			});
			expect(instanceTypeSelect).toBeInTheDocument();

			await waitFor(async () => {
				await userEvent.click(instanceTypeSelect);
			});

			let mediumOption: Element | null = null;
			await waitFor(() => {
				mediumOption = screen.queryByRole("option", { name: /t3\.medium/i });
				expect(mediumOption).toBeTruthy();
			});

			await waitFor(async () => {
				await userEvent.click(mediumOption!);
			});

			expect(mockWebSocket.send).toHaveBeenCalledWith(
				expect.stringContaining('"instance_type":"t3.medium"'),
			);
		});

		it("handles WebSocket error gracefully", async () => {
			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((versionId, _ownerId, callbacks) => {
					const [socket, pub] = createMockWebSocket(`ws://test/${versionId}`);
					mockWebSocket = socket;
					publisher = pub;

					socket.addEventListener("error", (event) => {
						callbacks.onError((event as ErrorEvent).error);
					});

					// Simulate error
					setTimeout(() => {
						publisher.publishError(
							new ErrorEvent("error", {
								error: new Error("Connection failed"),
							}),
						);
					}, 10);

					return mockWebSocket;
				});

			renderCreateWorkspacePageExperimental();

			await waitFor(() => {
				expect(screen.getByText(/connection failed/i)).toBeInTheDocument();
			});
		});

		it("handles WebSocket close event", async () => {
			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((versionId, _ownerId, callbacks) => {
					const [socket, pub] = createMockWebSocket(`ws://test/${versionId}`);
					mockWebSocket = socket;
					publisher = pub;

					socket.addEventListener("close", () => {
						callbacks.onClose();
					});

					setTimeout(() => {
						publisher.publishClose(new CloseEvent("close"));
					}, 10);

					return mockWebSocket;
				});

			renderCreateWorkspacePageExperimental();

			await waitFor(() => {
				expect(
					screen.getByText(/websocket connection.*unexpectedly closed/i),
				).toBeInTheDocument();
			});
		});

		it("only parameteres from latest reponse are displayed", async () => {
			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((versionId, _ownerId, callbacks) => {
					const [socket, pub] = createMockWebSocket(`ws://test/${versionId}`);
					mockWebSocket = socket;
					publisher = pub;

					socket.addEventListener("message", (event) => {
						callbacks.onMessage(JSON.parse(event.data));
					});

					// Establish connection and send initial parameters
					setTimeout(() => {
					publisher.publishOpen(new Event("open"));
					publisher.publishMessage(
						new MessageEvent("message", {
							data: JSON.stringify({
								id: 0,
								parameters: [mockStringParameter],
								diagnostics: [],
							}),
						}),
					);
				}, 0);

					return mockWebSocket;
				});

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			const response1: DynamicParametersResponse = {
				id: 1,
				parameters: [mockStringParameter],
				diagnostics: [],
			};
			const response2: DynamicParametersResponse = {
				id: 4,
				parameters: [mockNumberParameter],
				diagnostics: [],
			};

			setTimeout(() => {
				publisher.publishMessage(
					new MessageEvent("message", { data: JSON.stringify(response1) }),
				);

				publisher.publishMessage(
					new MessageEvent("message", { data: JSON.stringify(response2) }),
				);
			}, 0);

			await waitFor(() => {
				expect(screen.queryByText("CPU Count")).toBeInTheDocument();
				expect(screen.queryByText("Instance Type")).not.toBeInTheDocument();
			});
		});
	});

	// describe("Dynamic Parameter Types", () => {
	// 	it("renders string parameter with select options", async () => {
	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		await waitFor(() => {
	// 			expect(screen.getByText("Instance Type")).toBeInTheDocument();
	// 			expect(
	// 				screen.getByRole("combobox", { name: /instance type/i }),
	// 			).toBeInTheDocument();
	// 		});

	// 		// Open select and verify options
	// 		const select = screen.getByRole("combobox", { name: /instance type/i });
	// 		await userEvent.click(select);

	// 		expect(screen.getByText("Small instance")).toBeInTheDocument();
	// 		expect(screen.getByText("Medium instance")).toBeInTheDocument();
	// 		expect(screen.getByText("Large instance")).toBeInTheDocument();
	// 	});

	// 	it("renders number parameter with slider", async () => {
	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		await waitFor(() => {
	// 			expect(screen.getByText("CPU Count")).toBeInTheDocument();
	// 			expect(
	// 				screen.getByRole("slider", { name: /cpu count/i }),
	// 			).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("renders boolean parameter with switch", async () => {
	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		await waitFor(() => {
	// 			expect(screen.getByText("Enable Monitoring")).toBeInTheDocument();
	// 			expect(
	// 				screen.getByRole("switch", { name: /enable monitoring/i }),
	// 			).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("renders list parameter with tag input", async () => {
	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		await waitFor(() => {
	// 			expect(screen.getByText("Tags")).toBeInTheDocument();
	// 			expect(
	// 				screen.getByRole("textbox", { name: /tags/i }),
	// 			).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("displays parameter validation errors", async () => {
	// 		jest
	// 			.spyOn(API, "templateVersionDynamicParameters")
	// 			.mockImplementation((versionId, _ownerId, callbacks) => {
	// 				const [socket, pub] = createMockWebSocket(`ws://test/${versionId}`);
	// 				mockWebSocket = socket;
	// 				publisher = pub;

	// 				socket.addEventListener("message", (event) => {
	// 					callbacks.onMessage(JSON.parse(event.data));
	// 				});

	// 				setTimeout(() => {
	// 					publisher.publishMessage(
	// 						new MessageEvent("message", {
	// 							data: JSON.stringify(mockDynamicParametersResponseWithError),
	// 						}),
	// 					);
	// 				}, 10);

	// 				return mockWebSocket;
	// 			});

	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		await waitFor(() => {
	// 			expect(screen.getByText("Validation failed")).toBeInTheDocument();
	// 			expect(
	// 				screen.getByText(
	// 					"The selected instance type is not available in this region",
	// 				),
	// 			).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("handles disabled parameters", async () => {
	// 		renderCreateWorkspacePageExperimental(
	// 			`/templates/${MockTemplate.name}/workspace?disable_params=instance_type,cpu_count`,
	// 		);
	// 		await waitForLoaderToBeRemoved();

	// 		await waitFor(() => {
	// 			const instanceTypeSelect = screen.getByRole("combobox", {
	// 				name: /instance type/i,
	// 			});
	// 			const cpuSlider = screen.getByRole("slider", { name: /cpu count/i });

	// 			expect(instanceTypeSelect).toBeDisabled();
	// 			expect(cpuSlider).toBeDisabled();
	// 		});
	// 	});
	// });

	// describe("External Authentication", () => {
	// 	it("displays external auth providers", async () => {
	// 		jest
	// 			.spyOn(API, "getTemplateVersionExternalAuth")
	// 			.mockResolvedValue([MockTemplateVersionExternalAuthGithub]);

	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		await waitFor(() => {
	// 			expect(screen.getByText(/github/i)).toBeInTheDocument();
	// 			expect(
	// 				screen.getByRole("button", { name: /connect/i }),
	// 			).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("shows authenticated state for connected providers", async () => {
	// 		jest
	// 			.spyOn(API, "getTemplateVersionExternalAuth")
	// 			.mockResolvedValue([
	// 				MockTemplateVersionExternalAuthGithubAuthenticated,
	// 			]);

	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		await waitFor(() => {
	// 			expect(screen.getByText(/github/i)).toBeInTheDocument();
	// 			expect(screen.getByText(/authenticated/i)).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("prevents auto-creation when required external auth is missing", async () => {
	// 		jest
	// 			.spyOn(API, "getTemplateVersionExternalAuth")
	// 			.mockResolvedValue([MockTemplateVersionExternalAuthGithub]);

	// 		renderCreateWorkspacePageExperimental(
	// 			`/templates/${MockTemplate.name}/workspace?mode=auto`,
	// 		);

	// 		await waitFor(() => {
	// 			expect(
	// 				screen.getByText(
	// 					/external authentication providers that are not connected/i,
	// 				),
	// 			).toBeInTheDocument();
	// 			expect(
	// 				screen.getByText(/auto-creation has been disabled/i),
	// 			).toBeInTheDocument();
	// 		});
	// 	});
	// });

	// describe("Auto-creation Mode", () => {
	// 	it("automatically creates workspace when all requirements are met", async () => {
	// 		jest
	// 			.spyOn(API, "getTemplateVersionExternalAuth")
	// 			.mockResolvedValue([
	// 				MockTemplateVersionExternalAuthGithubAuthenticated,
	// 			]);

	// 		renderCreateWorkspacePageExperimental(
	// 			`/templates/${MockTemplate.name}/workspace?mode=auto&name=test-workspace`,
	// 		);

	// 		await waitFor(() => {
	// 			expect(API.createWorkspace).toHaveBeenCalledWith(
	// 				expect.objectContaining({
	// 					name: "test-workspace",
	// 					template_version_id: MockTemplate.active_version_id,
	// 				}),
	// 			);
	// 		});
	// 	});

	// 	it("falls back to form mode when auto-creation fails", async () => {
	// 		jest
	// 			.spyOn(API, "getTemplateVersionExternalAuth")
	// 			.mockResolvedValue([
	// 				MockTemplateVersionExternalAuthGithubAuthenticated,
	// 			]);
	// 		jest
	// 			.spyOn(API, "createWorkspace")
	// 			.mockRejectedValue(new Error("Auto-creation failed"));

	// 		renderCreateWorkspacePageExperimental(
	// 			`/templates/${MockTemplate.name}/workspace?mode=auto`,
	// 		);

	// 		await waitFor(() => {
	// 			expect(screen.getByText("Create workspace")).toBeInTheDocument();
	// 			expect(
	// 				screen.getByRole("button", { name: /create workspace/i }),
	// 			).toBeInTheDocument();
	// 		});
	// 	});
	// });

	// describe("Form Submission", () => {
	// 	it("creates workspace with correct parameters", async () => {
	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		// Wait for form to load
	// 		await waitFor(() => {
	// 			expect(screen.getByText("Instance Type")).toBeInTheDocument();
	// 		});

	// 		// Fill in workspace name
	// 		const nameInput = screen.getByRole("textbox", {
	// 			name: /workspace name/i,
	// 		});
	// 		await userEvent.clear(nameInput);
	// 		await userEvent.type(nameInput, "my-test-workspace");

	// 		// Submit form
	// 		const createButton = screen.getByRole("button", {
	// 			name: /create workspace/i,
	// 		});
	// 		await userEvent.click(createButton);

	// 		await waitFor(() => {
	// 			expect(API.createWorkspace).toHaveBeenCalledWith(
	// 				expect.objectContaining({
	// 					name: "my-test-workspace",
	// 					template_version_id: MockTemplate.active_version_id,
	// 					user_id: MockUserOwner.id,
	// 				}),
	// 			);
	// 		});
	// 	});

	// 	it("displays creation progress", async () => {
	// 		jest
	// 			.spyOn(API, "createWorkspace")
	// 			.mockImplementation(
	// 				() =>
	// 					new Promise((resolve) =>
	// 						setTimeout(() => resolve(MockWorkspace), 1000),
	// 					),
	// 			);

	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		const nameInput = screen.getByRole("textbox", {
	// 			name: /workspace name/i,
	// 		});
	// 		await userEvent.clear(nameInput);
	// 		await userEvent.type(nameInput, "my-test-workspace");

	// 		// Submit form
	// 		const createButton = screen.getByRole("button", {
	// 			name: /create workspace/i,
	// 		});
	// 		await userEvent.click(createButton);

	// 		// Should show loading state
	// 		expect(screen.getByText(/creating/i)).toBeInTheDocument();
	// 		expect(createButton).toBeDisabled();
	// 	});

	// 	it("handles creation errors", async () => {
	// 		const errorMessage = "Failed to create workspace";
	// 		jest
	// 			.spyOn(API, "createWorkspace")
	// 			.mockRejectedValue(new Error(errorMessage));

	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		const nameInput = screen.getByRole("textbox", {
	// 			name: /workspace name/i,
	// 		});
	// 		await userEvent.clear(nameInput);
	// 		await userEvent.type(nameInput, "my-test-workspace");

	// 		// Submit form
	// 		const createButton = screen.getByRole("button", {
	// 			name: /create workspace/i,
	// 		});
	// 		await userEvent.click(createButton);

	// 		await waitFor(() => {
	// 			expect(screen.getByText(errorMessage)).toBeInTheDocument();
	// 		});
	// 	});
	// });

	// describe("URL Parameters", () => {
	// 	it("pre-fills parameters from URL", async () => {
	// 		renderCreateWorkspacePageExperimental(
	// 			`/templates/${MockTemplate.name}/workspace?param.instance_type=t3.large&param.cpu_count=4`,
	// 		);
	// 		await waitForLoaderToBeRemoved();

	// 		await waitFor(() => {
	// 			// Verify parameters are pre-filled
	// 			// This would require checking the actual form values
	// 			expect(screen.getByText("Instance Type")).toBeInTheDocument();
	// 			expect(screen.getByText("CPU Count")).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("uses custom template version when specified", async () => {
	// 		const customVersionId = "custom-version-123";

	// 		renderCreateWorkspacePageExperimental(
	// 			`/templates/${MockTemplate.name}/workspace?version=${customVersionId}`,
	// 		);

	// 		await waitFor(() => {
	// 			expect(API.templateVersionDynamicParameters).toHaveBeenCalledWith(
	// 				customVersionId,
	// 				MockUserOwner.id,
	// 				expect.any(Object),
	// 			);
	// 		});
	// 	});

	// 	it("pre-fills workspace name from URL", async () => {
	// 		const workspaceName = "my-custom-workspace";

	// 		renderCreateWorkspacePageExperimental(
	// 			`/templates/${MockTemplate.name}/workspace?name=${workspaceName}`,
	// 		);
	// 		await waitForLoaderToBeRemoved();

	// 		await waitFor(() => {
	// 			const nameInput = screen.getByRole("textbox", {
	// 				name: /workspace name/i,
	// 			});
	// 			expect(nameInput).toHaveValue(workspaceName);
	// 		});
	// 	});
	// });

	// describe("Template Presets", () => {
	// 	const mockPreset = {
	// 		ID: "preset-1",
	// 		Name: "Development",
	// 		description: "Development environment preset",
	// 		Parameters: [
	// 			{ Name: "instance_type", Value: "t3.small" },
	// 			{ Name: "cpu_count", Value: "2" },
	// 		],
	// 		Default: false,
	// 	};

	// 	it("displays available presets", async () => {
	// 		jest
	// 			.spyOn(API, "getTemplateVersionPresets")
	// 			.mockResolvedValue([mockPreset]);

	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		await waitFor(() => {
	// 			expect(screen.getByText("Development")).toBeInTheDocument();
	// 			expect(
	// 				screen.getByText("Development environment preset"),
	// 			).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("applies preset parameters when selected", async () => {
	// 		jest
	// 			.spyOn(API, "getTemplateVersionPresets")
	// 			.mockResolvedValue([mockPreset]);

	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		// Select preset
	// 		const presetButton = screen.getByRole("button", { name: /development/i });
	// 		await userEvent.click(presetButton);

	// 		// Verify parameters are sent via WebSocket
	// 		await waitFor(() => {
	// 			expect(mockWebSocket.send).toHaveBeenCalledWith(
	// 				expect.stringContaining('"instance_type":"t3.small"'),
	// 			);
	// 			expect(mockWebSocket.send).toHaveBeenCalledWith(
	// 				expect.stringContaining('"cpu_count":"2"'),
	// 			);
	// 		});
	// 	});
	// });

	// describe("Navigation", () => {
	// 	it("navigates back when cancel is clicked", async () => {
	// 		const { router } = renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		const cancelButton = screen.getByRole("button", { name: /cancel/i });
	// 		await userEvent.click(cancelButton);

	// 		expect(router.state.location.pathname).not.toBe(
	// 			`/templates/${MockTemplate.name}/workspace`,
	// 		);
	// 	});

	// 	it("navigates to workspace after successful creation", async () => {
	// 		const { router } = renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		const nameInput = screen.getByRole("textbox", {
	// 			name: /workspace name/i,
	// 		});
	// 		await userEvent.clear(nameInput);
	// 		await userEvent.type(nameInput, "my-test-workspace");

	// 		// Submit form
	// 		const createButton = screen.getByRole("button", {
	// 			name: /create workspace/i,
	// 		});
	// 		await userEvent.click(createButton);

	// 		await waitFor(() => {
	// 			expect(router.state.location.pathname).toBe(
	// 				`/@${MockWorkspace.owner_name}/${MockWorkspace.name}`,
	// 			);
	// 		});
	// 	});
	// });

	// describe("Error Handling", () => {
	// 	it("displays template loading errors", async () => {
	// 		const errorMessage = "Template not found";
	// 		jest.spyOn(API, "getTemplate").mockRejectedValue(new Error(errorMessage));

	// 		renderCreateWorkspacePageExperimental();

	// 		await waitFor(() => {
	// 			expect(screen.getByText(errorMessage)).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("displays permission errors", async () => {
	// 		const errorMessage = "Insufficient permissions";
	// 		jest
	// 			.spyOn(API, "checkAuthorization")
	// 			.mockRejectedValue(new Error(errorMessage));

	// 		renderCreateWorkspacePageExperimental();

	// 		await waitFor(() => {
	// 			expect(screen.getByText(errorMessage)).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("allows error reset", async () => {
	// 		const errorMessage = "Creation failed";
	// 		jest
	// 			.spyOn(API, "createWorkspace")
	// 			.mockRejectedValue(new Error(errorMessage));

	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		// Trigger error
	// 		const createButton = screen.getByRole("button", {
	// 			name: /create workspace/i,
	// 		});
	// 		await userEvent.click(createButton);

	// 		await waitFor(() => {
	// 			expect(screen.getByText(errorMessage)).toBeInTheDocument();
	// 		});

	// 		// Reset error
	// 		jest.spyOn(API, "createWorkspace").mockResolvedValue(MockWorkspace);
	// 		const errorBanner = screen.getByRole("alert");
	// 		const tryAgainButton = within(errorBanner).getByRole("button", {
	// 			name: /try again/i,
	// 		});
	// 		await userEvent.click(tryAgainButton);

	// 		await waitFor(() => {
	// 			expect(screen.queryByText(errorMessage)).not.toBeInTheDocument();
	// 		});
	// 	});
	// });
});
