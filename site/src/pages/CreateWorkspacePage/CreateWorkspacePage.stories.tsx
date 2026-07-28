import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	MockTemplate,
	MockTemplateVersion,
	MockTemplateVersionExternalAuthAzure,
	MockTemplateVersionExternalAuthGithub,
	MockTemplateVersionExternalAuthGithubAuthenticated,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import CreateWorkspacePage from "./CreateWorkspacePage";

/**
 * Mocks API.templateVersionDynamicParameters to immediately send an empty
 * DynamicParametersResponse so the page renders the form instead of the
 * loader.
 */
function mockDynamicParameters() {
	spyOn(API, "templateVersionDynamicParameters").mockImplementation(
		(_versionId, _ownerId, callbacks) => {
			// Fire asynchronously so the component mounts before the message
			// arrives, matching real WebSocket behavior.
			setTimeout(() => {
				callbacks.onMessage({ id: 0, parameters: [], diagnostics: [] });
			}, 0);

			return { close: () => {} } as unknown as WebSocket;
		},
	);
}

const meta: Meta<typeof CreateWorkspacePage> = {
	title: "pages/CreateWorkspacePage",
	component: CreateWorkspacePage,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					organization: MockTemplate.organization_name,
					template: MockTemplate.name,
				},
			},
			routing: {
				path: "/templates/:organization/:template/workspace",
			},
		}),
	},
	beforeEach: () => {
		// Prevent the auth button from actually opening a popup.
		spyOn(window, "open").mockReturnValue(null);

		// Template, version, and preset queries.
		spyOn(API, "getTemplateByName").mockResolvedValue(MockTemplate);
		spyOn(API, "getTemplateVersion").mockResolvedValue(MockTemplateVersion);
		spyOn(API, "getTemplateVersionPresets").mockResolvedValue(null);
		spyOn(API, "checkAuthorization").mockResolvedValue({
			createWorkspaceForAny: true,
			canUpdateTemplate: false,
		});

		// Dynamic parameters over WebSocket.
		mockDynamicParameters();

		// Default: no external auth required.
		spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([]);
	},
};

export default meta;
type Story = StoryObj<typeof CreateWorkspacePage>;

/**
 * Renders two unauthenticated external auth providers. Both "Login with"
 * buttons should be visible and enabled.
 */
export const MultipleExternalAuth: Story = {
	// TODO: This story fails when pixel runs its play function. Fix it and remove the exclude.
	parameters: { pixel: { exclude: true } },
	beforeEach: () => {
		spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
			MockTemplateVersionExternalAuthGithub,
			MockTemplateVersionExternalAuthAzure,
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const githubButton = await canvas.findByRole("button", {
			name: /login with github/i,
		});
		const azureButton = await canvas.findByRole("button", {
			name: /login with azure/i,
		});

		expect(githubButton).toBeEnabled();
		expect(azureButton).toBeEnabled();
	},
};

/**
 * Clicking one external auth button should only show a loading spinner on
 * that button. The other provider's button must remain enabled so the user
 * can authenticate with both without a page refresh.
 *
 * This is the regression test for coder/coder#22420.
 */
export const ClickingOneAuthDoesNotDisableOthers: Story = {
	// TODO: This story fails when pixel runs its play function. Fix it and remove the exclude.
	parameters: { pixel: { exclude: true } },
	beforeEach: () => {
		spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
			MockTemplateVersionExternalAuthGithub,
			MockTemplateVersionExternalAuthAzure,
		]);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		const githubButton = await canvas.findByRole("button", {
			name: /login with github/i,
		});
		const azureButton = await canvas.findByRole("button", {
			name: /login with azure/i,
		});

		await step("Click GitHub auth button", async () => {
			await userEvent.click(githubButton);
		});

		await step("Azure button remains enabled", () => {
			expect(azureButton).toBeEnabled();
		});
	},
};

/**
 * After the first provider completes authentication and the API starts
 * returning it as authenticated, its button should be replaced with the
 * "Authenticated" badge. The second provider's button should still be
 * clickable.
 */
export const OneProviderAuthenticated: Story = {
	// TODO: This story fails when pixel runs its play function. Fix it and remove the exclude.
	parameters: { pixel: { exclude: true } },
	beforeEach: () => {
		spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
			MockTemplateVersionExternalAuthGithubAuthenticated,
			MockTemplateVersionExternalAuthAzure,
		]);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("GitHub shows authenticated", async () => {
			await canvas.findByText("Authenticated");
		});

		await step("Azure login button is still enabled", async () => {
			const azureButton = await canvas.findByRole("button", {
				name: /login with azure/i,
			});
			expect(azureButton).toBeEnabled();
		});
	},
};

/**
 * Simulates the full two-provider authentication flow: click the first
 * provider, have polling return it as authenticated, then click the second
 * provider.
 */
export const SequentialAuthFlow: Story = {
	// TODO: This story fails when pixel runs its play function. Fix it and remove the exclude.
	parameters: { pixel: { exclude: true } },
	beforeEach: () => {
		// First call: both unauthenticated.
		// Subsequent calls: GitHub authenticated (simulating a successful login
		// during the polling interval).
		spyOn(API, "getTemplateVersionExternalAuth")
			.mockResolvedValueOnce([
				MockTemplateVersionExternalAuthGithub,
				MockTemplateVersionExternalAuthAzure,
			])
			.mockResolvedValue([
				MockTemplateVersionExternalAuthGithubAuthenticated,
				MockTemplateVersionExternalAuthAzure,
			]);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("Both buttons render initially", async () => {
			await canvas.findByRole("button", { name: /login with github/i });
			await canvas.findByRole("button", { name: /login with azure/i });
		});

		await step("Click GitHub and wait for it to authenticate", async () => {
			const githubButton = await canvas.findByRole("button", {
				name: /login with github/i,
			});
			await userEvent.click(githubButton);

			// Polling picks up the updated mock that returns GitHub as
			// authenticated. The "Authenticated" text replaces the button.
			await waitFor(() => {
				expect(
					canvas.queryByRole("button", { name: /login with github/i }),
				).not.toBeInTheDocument();
			});
		});

		await step("Azure button is still clickable", async () => {
			const azureButton = await canvas.findByRole("button", {
				name: /login with azure/i,
			});
			expect(azureButton).toBeEnabled();
		});
	},
};
