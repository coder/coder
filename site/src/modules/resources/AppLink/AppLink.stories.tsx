import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { API } from "#/api/api";
import { getPreferredProxy } from "#/contexts/ProxyContext";
import {
	MockPrimaryWorkspaceProxy,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceProxies,
} from "#/testHelpers/entities";
import { withProxyProvider, withToaster } from "#/testHelpers/storybook";
import { AppLink } from "./AppLink";

const meta: Meta<typeof AppLink> = {
	title: "modules/resources/AppLink",
	component: AppLink,
	decorators: [
		withProxyProvider({
			proxy: {
				...getPreferredProxy(MockWorkspaceProxies, MockPrimaryWorkspaceProxy),
				preferredWildcardHostname: "*.super_proxy.tld",
			},
		}),
	],
};

export default meta;
type Story = StoryObj<typeof AppLink>;

export const WithIcon: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			icon: "/icon/code.svg",
			sharing_level: "owner",
			health: "healthy",
		},
		agent: MockWorkspaceAgent,
	},
};

export const WithNonSquaredIcon: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			icon: "/icon/windsurf.svg",
			sharing_level: "owner",
			health: "healthy",
		},
		agent: MockWorkspaceAgent,
	},
};

export const ExternalApp: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			url: "vscode://open",
			external: true,
		},
		agent: MockWorkspaceAgent,
	},
};

export const ExternalAppNotInstalled: Story = {
	decorators: [withToaster],
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			external: true,
			url: "foobar-foobaz://open-me",
		},
		agent: MockWorkspaceAgent,
	},
};

export const ExternalAppShareable: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			url: "vscode://open",
			external: true,
			sharing_level: "authenticated",
		},
		agent: MockWorkspaceAgent,
	},
};

export const InvalidExternalAppUrl: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			external: true,
			// A bare string with no scheme is unparsable by the URL constructor.
			url: "my-repo",
		},
		agent: MockWorkspaceAgent,
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		// A disabled app renders an anchor without an href, which has no
		// "link" role, so query by its label text instead.
		const trigger = await canvas.findByText("Test App");
		// The disabled button sets `pointer-events: none`, so bypass the
		// pointer-events guard to hover and reveal the tooltip.
		const user = userEvent.setup({ pointerEventsCheck: 0 });

		await step("button is disabled", async () => {
			const anchor = trigger.closest("a");
			expect(anchor).not.toBeNull();
			expect(anchor).not.toHaveAttribute("href");
		});

		await step("tooltip explains the invalid URL", async () => {
			await user.hover(trigger);
			const tooltip = await screen.findByRole("tooltip");
			expect(tooltip).toHaveTextContent(
				"This app has an invalid URL and can't be opened.",
			);
		});
	},
};

export const SharingLevelOwner: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			sharing_level: "owner",
		},
		agent: MockWorkspaceAgent,
	},
};

export const SharingLevelAuthenticated: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			sharing_level: "authenticated",
		},
		agent: MockWorkspaceAgent,
	},
};

export const SharingLevelOrganization: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			sharing_level: "organization",
		},
		agent: MockWorkspaceAgent,
	},
};

export const SharingLevelPublic: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			sharing_level: "public",
		},
		agent: MockWorkspaceAgent,
	},
};

export const HealthDisabled: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			sharing_level: "owner",
			health: "disabled",
		},
		agent: MockWorkspaceAgent,
	},
};

export const HealthInitializing: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			health: "initializing",
		},
		agent: MockWorkspaceAgent,
	},
};

export const HealthUnhealthy: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			health: "unhealthy",
		},
		agent: MockWorkspaceAgent,
	},
};

export const InternalApp: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			display_name: "Check my URL",
			subdomain: true,
			subdomain_name: "slug--agent_name--workspace_name--username",
		},
		agent: MockWorkspaceAgent,
	},
};

export const InternalAppHostnameTooLong: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			display_name: "Check my URL",
			subdomain: true,
			subdomain_name:
				// 64 characters long; surpasses DNS hostname limit of 63 characters
				"app_name_makes_subdomain64--agent_name--workspace_name--username",
		},
		agent: MockWorkspaceAgent,
	},
};

export const BlockingStartupScriptRunning: Story = {
	args: {
		workspace: MockWorkspace,
		app: MockWorkspaceApp,
		agent: {
			...MockWorkspaceAgent,
			lifecycle_state: "starting",
			startup_script_behavior: "blocking",
		},
	},
};

export const WithTooltip: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			tooltip:
				"This is a tooltip with Markdown: **bold**, _italic_, and [link](https://coder.com/docs)",
		},
		agent: MockWorkspaceAgent,
	},
};

// Regression test for DEVEX-460: external apps that embed the session token
// must not mint an API key on render. The key is minted only when the user
// clicks the link.
export const ExternalAppDefersSessionToken: Story = {
	decorators: [withToaster],
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			external: true,
			url: "jetbrains-gateway://connect?token=$SESSION_TOKEN",
		},
		agent: MockWorkspaceAgent,
	},
	play: async ({ canvasElement, step }) => {
		// Never resolve: we only assert whether/when the request fires, and
		// leaving it pending avoids the subsequent protocol-handler navigation.
		const getApiKey = spyOn(API, "getApiKey").mockImplementation(
			() => new Promise(() => {}),
		);
		const canvas = within(canvasElement);
		const link = await canvas.findByRole("link");
		const user = userEvent.setup();

		await step("no API key is minted on render", async () => {
			expect(getApiKey).not.toHaveBeenCalled();
		});

		await step("clicking mints the API key on demand", async () => {
			await user.click(link);
			await waitFor(() => expect(getApiKey).toHaveBeenCalledTimes(1));
		});
	},
};

// External apps that do not embed the session token must never mint a key,
// even on click, so we don't create session keys for apps that don't need one.
export const ExternalAppWithoutSessionTokenNeverMints: Story = {
	decorators: [withToaster],
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			external: true,
			url: "https://example.com",
			open_in: "slim-window",
		},
		agent: MockWorkspaceAgent,
	},
	play: async ({ canvasElement, step }) => {
		const getApiKey = spyOn(API, "getApiKey").mockResolvedValue({
			key: "test-key",
		});
		// The app opens in a slim window, so stub window.open to keep the click
		// from navigating the test frame.
		spyOn(window, "open").mockReturnValue(null);
		const canvas = within(canvasElement);
		const link = await canvas.findByRole("link");
		const user = userEvent.setup();

		await step("no API key is minted on render or click", async () => {
			expect(getApiKey).not.toHaveBeenCalled();
			await user.click(link);
			expect(getApiKey).not.toHaveBeenCalled();
		});
	},
};

export const SlimWindowPopupBlocked: Story = {
	decorators: [withToaster],
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			open_in: "slim-window",
		},
		agent: MockWorkspaceAgent,
	},
	play: async ({ canvasElement }) => {
		spyOn(window, "open").mockReturnValue(null);
		const canvas = within(canvasElement);
		const link = await canvas.findByRole("link");
		const user = userEvent.setup();
		await user.click(link);
		const toastMessage = await screen.findByText(
			"Popup blocked. Allow popups to open this app.",
		);
		expect(toastMessage).toBeInTheDocument();
	},
};
