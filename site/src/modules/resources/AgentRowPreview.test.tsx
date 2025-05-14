import { screen } from "@testing-library/react";
import {
	type DisplayApp,
	DisplayApps,
	type WorkspaceAgent,
} from "api/typesGenerated";
import { MockWorkspaceAgent } from "testHelpers/entities";
import { renderComponent } from "testHelpers/renderHelpers";
import { AgentRowPreview } from "./AgentRowPreview";
import { DisplayAppNameMap } from "./AppLink/AppLink";

const AllDisplayAppsAndModule = MockWorkspaceAgent;
const VSCodeNoInsiders = {
	...MockWorkspaceAgent,
	display_apps: [
		"ssh_helper",
		"port_forwarding_helper",
		"vscode",
		"web_terminal",
	] as DisplayApp[],
};
const VSCodeWithInsiders = {
	...MockWorkspaceAgent,
	display_apps: [
		"ssh_helper",
		"port_forwarding_helper",
		"vscode",
		"vscode_insiders",
		"web_terminal",
	] as DisplayApp[],
};
const NoVSCode = {
	...MockWorkspaceAgent,
	display_apps: [
		"ssh_helper",
		"port_forwarding_helper",
		"web_terminal",
	] as DisplayApp[],
};

const NoModulesJustApps = {
	...MockWorkspaceAgent,
	apps: [],
};

const NoAppsJustModules = {
	...MockWorkspaceAgent,
	display_apps: [] as DisplayApp[],
};

const EmptyAppPreview = {
	...MockWorkspaceAgent,
	apps: [],
	display_apps: [] as DisplayApp[],
};

describe("AgentRowPreviewApps", () => {
	it.each<{
		workspaceAgent: WorkspaceAgent;
		testName: string;
	}>([
		{
			workspaceAgent: AllDisplayAppsAndModule,
			testName: "AllDisplayAppsAndModule",
		},
		{
			workspaceAgent: VSCodeNoInsiders,
			testName: "VSCodeNoInsiders",
		},
		{
			workspaceAgent: VSCodeWithInsiders,
			testName: "VSCodeWithInsiders",
		},
		{
			workspaceAgent: NoVSCode,
			testName: "NoVSCode",
		},
		{
			workspaceAgent: NoModulesJustApps,
			testName: "NoModulesJustApps",
		},
		{
			workspaceAgent: NoAppsJustModules,
			testName: "NoAppsJustModules",
		},
		{
			workspaceAgent: EmptyAppPreview,
			testName: "EmptyAppPreview",
		},
	])(
		"<AgentRowPreview agent={$testName} /> displays appropriately",
		({ workspaceAgent }) => {
			renderComponent(<AgentRowPreview agent={workspaceAgent} />);
			for (const app of workspaceAgent.apps) {
				expect(
					screen.getByText(app.display_name as string),
				).toBeInTheDocument();
			}

			for (const app of workspaceAgent.display_apps) {
				// These get special treatment
				if (app === "vscode" || app === "vscode_insiders") {
					continue;
				}
				expect(screen.getByText(DisplayAppNameMap[app])).toBeInTheDocument();
			}

			// test VS Code display
			if (workspaceAgent.display_apps.includes("vscode")) {
				expect(screen.getByText(DisplayAppNameMap.vscode)).toBeInTheDocument();
			} else if (workspaceAgent.display_apps.includes("vscode_insiders")) {
				expect(
					screen.getByText(DisplayAppNameMap.vscode_insiders),
				).toBeInTheDocument();
			} else {
				expect(screen.queryByText("vscode")).not.toBeInTheDocument();
				expect(screen.queryByText("vscode_insiders")).not.toBeInTheDocument();
			}

			// difference between all possible display apps and those displayed
			const excludedApps = DisplayApps.filter(
				(a) => !workspaceAgent.display_apps.includes(a),
			);

			for (const app of excludedApps) {
				expect(
					screen.queryByText(DisplayAppNameMap[app]),
				).not.toBeInTheDocument();
			}

			// test empty state
			if (
				workspaceAgent.display_apps.length === 0 &&
				workspaceAgent.apps.length === 0
			) {
				expect(screen.getByText("None")).toBeInTheDocument();
			}
		},
	);
});
