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

const portPreview: WorkspacePreviewRightPanelTab = {
	id: "port-1",
	kind: "port",
	label: ":3000",
	agentId: agent.id,
	port: 3000,
	protocol: "http",
};

const secondPortPreview: WorkspacePreviewRightPanelTab = {
	...portPreview,
	id: "port-2",
	label: ":5173",
	port: 5173,
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

describe("WorkspaceTabPanel", () => {
	it("opens an embeddable app from the empty state", async () => {
		const user = userEvent.setup();
		const { onOpenWorkspaceApp } = renderPanel();

		await user.click(screen.getByRole("button", { name: "Open app or port" }));
		await user.click(
			await screen.findByRole("menuitem", { name: "code-server" }),
		);

		expect(onOpenWorkspaceApp).toHaveBeenCalledWith(embeddableApp);
	});

	it("routes command apps to the terminal handler", async () => {
		const user = userEvent.setup();
		const { onOpenCommandApp, onOpenWorkspaceApp } = renderPanel();

		await user.click(screen.getByRole("button", { name: "Open app or port" }));
		await user.click(
			await screen.findByRole("menuitem", { name: "Claude Code" }),
		);

		expect(onOpenCommandApp).toHaveBeenCalledWith(commandApp);
		expect(onOpenWorkspaceApp).not.toHaveBeenCalled();
	});

	it("switches and closes preview chips", async () => {
		const user = userEvent.setup();
		const { onActivePreviewChange, onClosePreview } = renderPanel({
			previews: [portPreview, secondPortPreview],
			activePreviewId: portPreview.id,
		});

		await user.click(screen.getByRole("tab", { name: ":5173" }));
		expect(onActivePreviewChange).toHaveBeenCalledWith(secondPortPreview.id);

		await user.click(screen.getByRole("button", { name: "Close :3000" }));
		expect(onClosePreview).toHaveBeenCalledWith(portPreview.id);
	});

	it("keeps the open menu available next to the chips", async () => {
		const user = userEvent.setup();
		const { onOpenWorkspaceApp } = renderPanel({
			previews: [portPreview],
			activePreviewId: portPreview.id,
		});

		await user.click(screen.getByRole("button", { name: "Open app or port" }));
		await user.click(
			await screen.findByRole("menuitem", { name: "code-server" }),
		);

		expect(onOpenWorkspaceApp).toHaveBeenCalledWith(embeddableApp);
	});
});
