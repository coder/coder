import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
	WorkspaceStatus,
} from "#/api/typesGenerated";
import {
	MockStoppedWorkspace,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceResource,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
} from "#/testHelpers/storybook";
import MuxPage from "./MuxPage";
import { getMuxCandidatesFromWorkspace, pickPreferredMuxApp } from "./muxApps";

const muxRouteParameters = (searchParams?: Record<string, string>) =>
	reactRouterParameters({
		location: { path: "/mux", searchParams },
		routing: { path: "/mux" },
	});

const routeWithWorkspace = (workspace: Workspace) =>
	muxRouteParameters({ workspace: workspace.id });

const meta = {
	title: "pages/MuxPage/MuxPage",
	component: MuxPage,
	decorators: [withAuthProvider, withDashboardProvider, withProxyProvider()],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		permissions: {
			viewDeploymentConfig: false,
		},
		reactRouter: muxRouteParameters(),
	},
} satisfies Meta<typeof MuxPage>;

export default meta;
type Story = StoryObj<typeof meta>;

const muxApp = (overrides: Partial<WorkspaceApp> = {}): WorkspaceApp => ({
	...MockWorkspaceApp,
	id: "mux-app",
	slug: "mux",
	display_name: "Mux",
	icon: "/icon/mux.svg",
	health: "healthy",
	open_in: "tab",
	...overrides,
});

type MakeWorkspaceOptions = {
	id: string;
	name: string;
	apps: readonly WorkspaceApp[];
	status?: WorkspaceStatus;
};

const workspaceWithApps = ({
	id,
	name,
	apps,
	status = "running",
}: MakeWorkspaceOptions): Workspace => {
	const baseWorkspace =
		status === "stopped" ? MockStoppedWorkspace : MockWorkspace;
	const agent: WorkspaceAgent = {
		...MockWorkspaceAgent,
		id: `${id}-agent`,
		name: "main",
		apps,
	};

	return {
		...baseWorkspace,
		id,
		name,
		owner_name: MockUserOwner.username,
		latest_build: {
			...baseWorkspace.latest_build,
			status,
			workspace_id: id,
			workspace_name: name,
			workspace_owner_name: MockUserOwner.username,
			resources: [
				{
					...MockWorkspaceResource,
					id: `${id}-resource`,
					agents: [agent],
				},
			],
		},
	};
};

const getPreferredMuxCandidate = (workspace: Workspace) => {
	const candidate = pickPreferredMuxApp(
		getMuxCandidatesFromWorkspace(workspace),
	);
	if (!candidate) {
		throw new Error(`Workspace ${workspace.name} does not have a Mux app.`);
	}
	return candidate;
};

const expectedMuxHref = (workspace: Workspace) => {
	const { agent, app } = getPreferredMuxCandidate(workspace);
	return `/@${workspace.owner_name}/${workspace.name}.${agent.name}/apps/${encodeURIComponent(app.slug)}/`;
};

const mockWorkspaces = (
	workspaces: readonly Workspace[],
	count = workspaces.length,
) => {
	spyOn(API, "getWorkspaces").mockResolvedValue({
		workspaces,
		count,
	});
};

const healthyWorkspace = workspaceWithApps({
	id: "workspace-alpha",
	name: "alpha",
	apps: [muxApp({ id: "mux-alpha", display_name: "Mux Alpha" })],
});

const secondHealthyWorkspace = workspaceWithApps({
	id: "workspace-beta",
	name: "beta",
	apps: [muxApp({ id: "mux-beta", display_name: "Mux Beta" })],
});

const stoppedWorkspace = workspaceWithApps({
	id: "workspace-stopped",
	name: "stopped",
	status: "stopped",
	apps: [muxApp({ id: "mux-stopped" })],
});

export const LoadingWorkspaces: Story = {
	beforeEach: () => {
		spyOn(API, "getWorkspaces").mockImplementation(() => new Promise(() => {}));
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByRole("status")).toHaveTextContent(/loading/i);
	},
};

export const EmptyNoWorkspaces: Story = {
	beforeEach: () => {
		mockWorkspaces([], 0);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("No workspaces yet")).toBeVisible();
	},
};

export const EmptyNoMuxWorkspaces: Story = {
	beforeEach: () => {
		mockWorkspaces([
			workspaceWithApps({
				id: "workspace-no-mux",
				name: "no-mux",
				apps: [
					{
						...MockWorkspaceApp,
						id: "code-app",
						slug: "code",
						icon: "/icon/code.svg",
					},
				],
			}),
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("No Mux workspaces found")).toBeVisible();
	},
};

export const MultipleMuxCandidates: Story = {
	beforeEach: () => {
		mockWorkspaces([healthyWorkspace, secondHealthyWorkspace]);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const user = userEvent.setup();

		await step("Show launcher until a workspace is selected", async () => {
			const launchButton = await canvas.findByRole("button", {
				name: /^launch$/i,
			});
			expect(launchButton).toBeDisabled();
			expect(await canvas.findByText("Select a workspace")).toBeVisible();
		});

		await step("Select the first workspace and launch", async () => {
			await user.click(
				await canvas.findByRole("button", { name: /select workspace/i }),
			);
			await user.click(await body.findByText(healthyWorkspace.name));

			const launchButton = await canvas.findByRole("button", {
				name: /^launch$/i,
			});
			expect(launchButton).toBeEnabled();
			expect(
				await canvas.findByRole("button", { name: /select workspace/i }),
			).toHaveTextContent(`${MockUserOwner.username}/${healthyWorkspace.name}`);

			await user.click(launchButton);

			const frame = await canvas.findByTestId("mux-iframe");
			expect(frame).toHaveAttribute("title", "Mux Alpha");
			expect(frame).toHaveAttribute("src", expectedMuxHref(healthyWorkspace));
			expect(
				await canvas.findByRole("button", { name: /change workspace/i }),
			).toBeVisible();
			expect(
				await canvas.findByRole("link", { name: /open in new tab/i }),
			).toHaveAttribute("href", expectedMuxHref(healthyWorkspace));
			expect(
				canvas.queryByRole("button", { name: /select workspace/i }),
			).not.toBeInTheDocument();
		});

		await step("Change workspace and launch the second workspace", async () => {
			await user.click(
				await canvas.findByRole("button", { name: /change workspace/i }),
			);
			expect(
				await canvas.findByRole("button", { name: /^launch$/i }),
			).toBeDisabled();

			await user.click(
				await canvas.findByRole("button", { name: /select workspace/i }),
			);
			await user.click(await body.findByText(secondHealthyWorkspace.name));
			await user.click(
				await canvas.findByRole("button", { name: /^launch$/i }),
			);

			await waitFor(() => {
				const frame = canvas.getByTestId("mux-iframe");
				expect(frame).toHaveAttribute("title", "Mux Beta");
				expect(frame).toHaveAttribute(
					"src",
					expectedMuxHref(secondHealthyWorkspace),
				);
			});
		});
	},
};

export const MultipleMuxAppsPreferDefaultSlug: Story = {
	parameters: {
		reactRouter: routeWithWorkspace(
			workspaceWithApps({
				id: "workspace-multiple-apps",
				name: "multiple-apps",
				apps: [muxApp()],
			}),
		),
	},
	beforeEach: () => {
		mockWorkspaces([
			workspaceWithApps({
				id: "workspace-multiple-apps",
				name: "multiple-apps",
				apps: [
					muxApp({
						id: "z-mux",
						slug: "z-mux",
						display_name: "Z Mux",
					}),
					muxApp({
						id: "default-mux",
						slug: "mux",
						display_name: "Preferred Mux",
					}),
				],
			}),
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const frame = await canvas.findByTestId("mux-iframe");
		expect(frame).toHaveAttribute("title", "Preferred Mux");
		expect(frame).toHaveAttribute(
			"src",
			`/@${MockUserOwner.username}/multiple-apps.main/apps/mux/`,
		);
	},
};

export const Launched: Story = {
	parameters: {
		reactRouter: routeWithWorkspace(healthyWorkspace),
	},
	beforeEach: () => {
		mockWorkspaces([healthyWorkspace]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const frame = await canvas.findByTestId("mux-iframe");
		const openLink = await canvas.findByRole("link", {
			name: /open in new tab/i,
		});

		expect(frame).toHaveAttribute("title", "Mux Alpha");
		expect(frame).toHaveAttribute("src", expectedMuxHref(healthyWorkspace));
		expect(openLink).toHaveAttribute("href", expectedMuxHref(healthyWorkspace));
		expect(
			await canvas.findByRole("button", { name: /change workspace/i }),
		).toBeVisible();
		expect(
			canvas.queryByRole("button", { name: /select workspace/i }),
		).not.toBeInTheDocument();
		expect(canvas.queryByText(/open the mux app/i)).not.toBeInTheDocument();
	},
};

export const SelectedStoppedWorkspace: Story = {
	beforeEach: () => {
		mockWorkspaces([stoppedWorkspace]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const user = userEvent.setup();

		await user.click(
			await canvas.findByRole("button", { name: /select workspace/i }),
		);
		await user.click(await body.findByText(stoppedWorkspace.name));

		const workspaceLink = await canvas.findByRole("link", {
			name: /open workspace/i,
		});

		expect(workspaceLink).toHaveAttribute(
			"href",
			`/@${MockUserOwner.username}/stopped`,
		);
		expect(canvas.queryByTestId("mux-iframe")).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: /^launch$/i }),
		).not.toBeInTheDocument();
		expect(await canvas.findByText("Start workspace to launch")).toBeVisible();
	},
};

export const MuxAppInitializing: Story = {
	parameters: {
		reactRouter: muxRouteParameters({ workspace: "workspace-initializing" }),
	},
	beforeEach: () => {
		mockWorkspaces([
			workspaceWithApps({
				id: "workspace-initializing",
				name: "initializing",
				apps: [muxApp({ id: "mux-initializing", health: "initializing" })],
			}),
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(await canvas.findByText("Mux is starting")).toBeVisible();
		expect(canvas.getByRole("status")).toHaveTextContent(
			/healthcheck to pass/i,
		);
	},
};

export const MuxAppUnhealthy: Story = {
	parameters: {
		reactRouter: muxRouteParameters({ workspace: "workspace-unhealthy" }),
	},
	beforeEach: () => {
		mockWorkspaces([
			workspaceWithApps({
				id: "workspace-unhealthy",
				name: "unhealthy",
				apps: [
					muxApp({
						id: "mux-unhealthy",
						health: "unhealthy",
						healthcheck: {
							url: "http://localhost:3000/healthz",
							interval: 10,
							threshold: 3,
						},
					}),
				],
			}),
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Mux app healthcheck is failing"),
		).toBeVisible();
		expect(
			await canvas.findByRole("link", { name: /open workspace logs/i }),
		).toHaveAttribute("href", `/@${MockUserOwner.username}/unhealthy`);
	},
};

export const SubdomainMissingWildcardHostname: Story = {
	parameters: {
		reactRouter: muxRouteParameters({ workspace: "workspace-subdomain" }),
	},
	beforeEach: () => {
		mockWorkspaces([
			workspaceWithApps({
				id: "workspace-subdomain",
				name: "subdomain",
				apps: [
					muxApp({
						id: "mux-subdomain",
						subdomain: true,
						subdomain_name: "mux--main--subdomain--admin",
					}),
				],
			}),
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText(
				"Workspace app wildcard hostname is not configured",
			),
		).toBeVisible();
		expect(canvas.queryByTestId("mux-iframe")).not.toBeInTheDocument();
	},
};
