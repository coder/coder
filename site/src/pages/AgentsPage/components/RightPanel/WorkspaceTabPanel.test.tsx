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
import { hasWorkspaceTabContent, WorkspaceTabPanel } from "./WorkspaceTabPanel";

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

const openMenu = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("button", { name: "Open an app or port" }));
};

describe("WorkspaceTabPanel", () => {
	it("opens an embeddable app that has no preview yet", async () => {
		const user = userEvent.setup();
		const { onOpenWorkspaceApp } = renderPanel();

		await openMenu(user);
		await user.click(
			await screen.findByRole("menuitemcheckbox", { name: "code-server" }),
		);

		expect(onOpenWorkspaceApp).toHaveBeenCalledWith(embeddableApp);
	});

	it("routes command apps to the terminal handler", async () => {
		const user = userEvent.setup();
		const { onOpenCommandApp, onOpenWorkspaceApp } = renderPanel();

		await openMenu(user);
		await user.click(
			await screen.findByRole("menuitem", { name: "Claude Code" }),
		);

		expect(onOpenCommandApp).toHaveBeenCalledWith(commandApp);
		expect(onOpenWorkspaceApp).not.toHaveBeenCalled();
	});

	it("switches and closes previews from their chips", async () => {
		const user = userEvent.setup();
		const { onActivePreviewChange, onClosePreview } = renderPanel({
			previews: [appPreview, portPreview],
			activePreviewId: portPreview.id,
		});

		await user.click(screen.getByRole("tab", { name: "code-server" }));
		expect(onActivePreviewChange).toHaveBeenCalledWith(appPreview.id);

		await user.click(screen.getByRole("button", { name: "Close :3000" }));
		expect(onClosePreview).toHaveBeenCalledWith(portPreview.id);
	});

	it("shows an already open app when picked from the add menu", async () => {
		const user = userEvent.setup();
		const { onActivePreviewChange, onOpenWorkspaceApp } = renderPanel({
			previews: [appPreview, portPreview],
			activePreviewId: portPreview.id,
		});

		await openMenu(user);
		await user.click(
			await screen.findByRole("menuitemcheckbox", { name: "code-server" }),
		);

		expect(onActivePreviewChange).toHaveBeenCalledWith(appPreview.id);
		expect(onOpenWorkspaceApp).not.toHaveBeenCalled();
	});
});

describe("hasWorkspaceTabContent", () => {
	it("is false when the agent has no visible apps and no port forwarding", () => {
		expect(
			hasWorkspaceTabContent({ ...agent, apps: [] }, "*.example.com"),
		).toBe(false);
		expect(
			hasWorkspaceTabContent(
				{ ...agent, apps: [{ ...embeddableApp, hidden: true }] },
				"*.example.com",
			),
		).toBe(false);
	});

	it("is true with a visible app or with port forwarding", () => {
		expect(hasWorkspaceTabContent(agent, "*.example.com")).toBe(true);
		expect(
			hasWorkspaceTabContent(
				{ ...agent, apps: [], display_apps: ["port_forwarding_helper"] },
				"*.example.com",
			),
		).toBe(true);
	});
});
