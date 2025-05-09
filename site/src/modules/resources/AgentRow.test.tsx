import { screen } from "@testing-library/react";
import {
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import type { AgentRowProps } from "./AgentRow";
import { AgentRow } from "./AgentRow";
import { DisplayAppNameMap } from "./AppLink/AppLink";

jest.mock("modules/resources/AgentMetadata", () => {
	const AgentMetadata = () => <></>;
	return { AgentMetadata };
});

describe.each<{
	result: "visible" | "hidden";
	props: Partial<AgentRowProps>;
}>([
	{
		result: "visible",
		props: {
			showApps: true,
			agent: {
				...MockWorkspaceAgent,
				display_apps: ["vscode", "vscode_insiders"],
				status: "connected",
			},
			hideVSCodeDesktopButton: false,
		},
	},
	{
		result: "hidden",
		props: {
			showApps: false,
			agent: {
				...MockWorkspaceAgent,
				display_apps: ["vscode", "vscode_insiders"],
				status: "connected",
			},
			hideVSCodeDesktopButton: false,
		},
	},
	{
		result: "hidden",
		props: {
			showApps: true,
			agent: {
				...MockWorkspaceAgent,
				display_apps: [],
				status: "connected",
			},
			hideVSCodeDesktopButton: false,
		},
	},
	{
		result: "hidden",
		props: {
			showApps: true,
			agent: {
				...MockWorkspaceAgent,
				display_apps: ["vscode", "vscode_insiders"],
				status: "disconnected",
			},
			hideVSCodeDesktopButton: false,
		},
	},
	{
		result: "hidden",
		props: {
			showApps: true,
			agent: {
				...MockWorkspaceAgent,
				display_apps: ["vscode", "vscode_insiders"],
				status: "connected",
			},
			hideVSCodeDesktopButton: true,
		},
	},
])("VSCode button visibility", ({ props: testProps, result }) => {
	const props: AgentRowProps = {
		agent: MockWorkspaceAgent,
		workspace: MockWorkspace,
		template: MockTemplate,
		showApps: false,
		serverVersion: "",
		serverAPIVersion: "",
		onUpdateAgent: () => {
			throw new Error("Function not implemented.");
		},
		...testProps,
	};

	test(`visibility: ${result}, showApps: ${props.showApps}, hideVSCodeDesktopButton: ${props.hideVSCodeDesktopButton}, display apps: ${props.agent.display_apps}`, async () => {
		renderWithAuth(<AgentRow {...props} />);
		await waitForLoaderToBeRemoved();

		if (result === "visible") {
			expect(screen.getByText(DisplayAppNameMap.vscode)).toBeVisible();
		} else {
			expect(screen.queryByText(DisplayAppNameMap.vscode)).toBeNull();
		}
	});
});

describe.each<{
	props: Partial<AgentRowProps>;
}>([
	{
		props: {
			agent: {
				...MockWorkspaceAgent,
				apps: [
					{
						...MockWorkspaceApp,
						display_name: `${MockWorkspaceApp.display_name} Not Hidden`,
						hidden: false,
					},
					{
						...MockWorkspaceApp,
						display_name: `${MockWorkspaceApp.display_name} Is Hidden`,
						hidden: true,
					},
				],
			},
		},
	},
])("hidden hides App button", ({ props: testProps }) => {
	const props: AgentRowProps = {
		agent: MockWorkspaceAgent,
		workspace: MockWorkspace,
		template: MockTemplate,
		showApps: true,
		serverVersion: "",
		serverAPIVersion: "",
		onUpdateAgent: () => {
			throw new Error("Function not implemented.");
		},
		...testProps,
	};

	test(`apps: ${props.agent.apps}`, async () => {
		renderWithAuth(<AgentRow {...props} />);
		await waitForLoaderToBeRemoved();

		for (const app of props.agent.apps) {
			if (app.hidden) {
				expect(screen.queryByText(app.display_name as string)).toBeNull();
			} else {
				expect(screen.getByText(app.display_name as string)).toBeVisible();
			}
		}
	});
});
