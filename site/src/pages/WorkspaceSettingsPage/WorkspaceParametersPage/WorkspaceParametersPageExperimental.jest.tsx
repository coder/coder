import { screen, waitFor, within } from "@testing-library/react";
import { act } from "react";
import { API } from "#/api/api";
import type { DynamicParametersResponse } from "#/api/typesGenerated";
import {
	MockPreviewParameter,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	MockTemplateVersionParameter4,
	MockValidationParameter,
	MockWorkspace,
	MockWorkspaceBuildParameter1,
	MockWorkspaceBuildParameter2,
	MockWorkspaceBuildParameter4,
} from "#/testHelpers/entities";
import {
	renderWithWorkspaceSettingsLayout,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import WorkspaceParametersPageExperimental from "./WorkspaceParametersPageExperimental";

// Minimal inline publisher for vitest. The shared websockets.ts helper
// uses jest globals and cannot be imported from vitest on this branch.
type Publisher = {
	publishMessage: (event: MessageEvent<string>) => void;
	publishOpen: (event: Event) => void;
};

function setupDynamicParameterWebSocket(
	response: DynamicParametersResponse,
): Publisher {
	const subs: Record<string, Set<(e: unknown) => void>> = {
		message: new Set(),
		error: new Set(),
		close: new Set(),
		open: new Set(),
	};

	const fakeSocket = {
		addEventListener: (type: string, cb: (e: unknown) => void) => {
			subs[type]?.add(cb);
		},
		removeEventListener: (type: string, cb: (e: unknown) => void) => {
			subs[type]?.delete(cb);
		},
		close: jest.fn(),
	};

	const publisher: Publisher = {
		publishOpen: (event) => {
			for (const sub of subs.open) sub(event);
		},
		publishMessage: (event) => {
			for (const sub of subs.message) sub(event);
		},
	};

	jest.spyOn(API, "templateVersionDynamicParameters").mockImplementation(
		(_versionId, _ownerId, callbacks) => {
			fakeSocket.addEventListener("message", (event) => {
				callbacks.onMessage(
					JSON.parse((event as MessageEvent<string>).data),
				);
			});
			fakeSocket.addEventListener("error", () => {
				callbacks.onError(
					new Error("Connection for dynamic parameters failed."),
				);
			});
			fakeSocket.addEventListener("close", () => {
				callbacks.onClose();
			});
			publisher.publishOpen(new Event("open"));
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify(response),
				}),
			);
			return fakeSocket as unknown as WebSocket;
		},
	);

	return publisher;
}

describe("WorkspaceParametersPageExperimental", () => {
	const renderWorkspaceParametersPageExperimental = (
		route = `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/settings`,
	) => {
		return renderWithWorkspaceSettingsLayout(
			<WorkspaceParametersPageExperimental />,
			{
				route,
				path: "/:username/:workspace/settings",
				extraRoutes: [
					{
						// Need this because after submit the user is redirected.
						path: "/:username/:workspace",
						element: <div>Workspace Page</div>,
					},
				],
			},
		);
	};

	beforeEach(() => {
		jest.clearAllMocks();
		jest.spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValueOnce(
			MockWorkspace,
		);
		jest.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValueOnce([
			MockTemplateVersionParameter1,
			MockTemplateVersionParameter2,
			MockTemplateVersionParameter4,
		]);
		jest.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValueOnce([
			MockWorkspaceBuildParameter1,
			MockWorkspaceBuildParameter2,
			MockWorkspaceBuildParameter4,
		]);
	});

	afterEach(() => {
		jest.useRealTimers();
		jest.restoreAllMocks();
	});

	it("does not clobber touched parameters", async () => {
		const publisher = setupDynamicParameterWebSocket({
			id: 0,
			parameters: [
				{
					...MockPreviewParameter,
					name: MockWorkspaceBuildParameter1.name,
				},
			],
			diagnostics: [],
		});

		renderWorkspaceParametersPageExperimental();
		await waitForLoaderToBeRemoved();

		// Simulate a stale response.
		await act(async () => {
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: 2,
						parameters: [
							{
								...MockPreviewParameter,
								name: MockWorkspaceBuildParameter1.name,
							},
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
			expect(
				within(form).getByDisplayValue(MockWorkspaceBuildParameter1.value),
			).toBeInTheDocument();
		});
	});
});
