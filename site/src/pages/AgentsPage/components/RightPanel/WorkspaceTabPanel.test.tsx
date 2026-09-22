import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { WorkspaceApp } from "#/api/typesGenerated";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import type { WorkspacePreviewRightPanelTab } from "../../utils/rightPanelTabs";
import { WorkspaceTabPanel } from "./WorkspaceTabPanel";

const embeddableApp: WorkspaceApp = {
	...MockWorkspaceApp,
	id: "code-server-app",
	slug: "code-server",
	display_name: "code-server",
	command: undefined,
	external: false,
	subdomain: false,
	health: "healthy",
};

const commandApp: WorkspaceApp = {
	...MockWorkspaceApp,
	id: "claude-app",
	slug: "claude",
	display_name: "Claude Code",
	command: "claude",
	health: "disabled",
};

const agent = {
	...MockWorkspaceAgent,
	apps: [embeddableApp, commandApp],
	// Leaves the ports submenu out so the menu needs no query client.
	display_apps: [],
};

const appPreview: WorkspacePreviewRightPanelTab = {
	id: "workspace_app-1",
	kind: "workspace_app",
	label: "code-server",
	agentId: agent.id,
	appId: embeddableApp.id,
};

const portPreview: WorkspacePreviewRightPanelTab = {
	id: "port-1",
	kind: "port",
	label: ":3000",
	agentId: agent.id,
	port: 3000,
	protocol: "http",
};

const renderPanel = (
	overrides: Partial<Parameters<typeof WorkspaceTabPanel>[0]> = {},
) => {
	const handlers = {
		onActivePreviewChange: vi.fn(),
		onClosePreview: vi.fn(),
		onOpenWorkspaceApp: vi.fn(),
		onOpenCommandApp: vi.fn(),
		onOpenPort: vi.fn(),
	};
	renderComponent(
		<WorkspaceTabPanel
			workspace={MockWorkspace}
			agent={agent}
			host="*.example.com"
			isRunning
			previews={[]}
			activePreviewId={null}
			isVisible
			{...handlers}
			{...overrides}
		/>,
	);
	return handlers;
};

const openSelector = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(
		screen.getByRole("button", {
			name: /open an app or port|code-server|:3000/i,
		}),
	);
};

describe("WorkspaceTabPanel", () => {
	it("opens an embeddable app that has no preview yet", async () => {
		const user = userEvent.setup();
		const { onOpenWorkspaceApp } = renderPanel();

		await openSelector(user);
		await user.click(
			await screen.findByRole("menuitemcheckbox", { name: "code-server" }),
		);

		expect(onOpenWorkspaceApp).toHaveBeenCalledWith(embeddableApp);
	});

	it("routes command apps to the terminal handler", async () => {
		const user = userEvent.setup();
		const { onOpenCommandApp, onOpenWorkspaceApp } = renderPanel();

		await openSelector(user);
		await user.click(
			await screen.findByRole("menuitem", { name: "Claude Code" }),
		);

		expect(onOpenCommandApp).toHaveBeenCalledWith(commandApp);
		expect(onOpenWorkspaceApp).not.toHaveBeenCalled();
	});

	it("switches to an open preview that is not shown", async () => {
		const user = userEvent.setup();
		const { onActivePreviewChange, onOpenWorkspaceApp } = renderPanel({
			previews: [appPreview, portPreview],
			activePreviewId: portPreview.id,
		});

		await openSelector(user);
		await user.click(
			await screen.findByRole("menuitemcheckbox", { name: "code-server" }),
		);

		expect(onActivePreviewChange).toHaveBeenCalledWith(appPreview.id);
		expect(onOpenWorkspaceApp).not.toHaveBeenCalled();
	});

	it("closes the preview that is currently shown", async () => {
		const user = userEvent.setup();
		const { onClosePreview, onActivePreviewChange } = renderPanel({
			previews: [appPreview, portPreview],
			activePreviewId: portPreview.id,
		});

		await openSelector(user);
		await user.click(
			await screen.findByRole("menuitemcheckbox", { name: ":3000" }),
		);

		expect(onClosePreview).toHaveBeenCalledWith(portPreview.id);
		expect(onActivePreviewChange).not.toHaveBeenCalled();
	});
});
