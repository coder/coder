import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { updateCheck } from "api/queries/updateCheck";
import { expect, userEvent, waitFor, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { DashboardLayout } from "./DashboardLayout";

const outletContent = <div>Dashboard content</div>;
const skipLinkName = "Skip to main content";
const outdatedUpdateCheck = {
	current: false,
	version: "v0.12.9",
	url: "https://github.com/coder/coder/releases/tag/v0.12.9",
};

const getMainContent = (): HTMLElement => {
	const mainContent = document.getElementById("main-content");
	if (!mainContent) {
		throw new Error("Expected #main-content to be rendered.");
	}

	return mainContent;
};

const meta = {
	title: "modules/dashboard/DashboardLayout",
	component: DashboardLayout,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		reactRouter: reactRouterParameters({
			routing: reactRouterOutlet({ path: "/" }, outletContent),
		}),
	},
} satisfies Meta<typeof DashboardLayout>;

export default meta;
type Story = StoryObj<typeof DashboardLayout>;

export const ShowsUpdateNotification: Story = {
	beforeEach: () => {
		localStorage.removeItem("dismissedVersion");
		return () => localStorage.removeItem("dismissedVersion");
	},
	parameters: {
		permissions: {
			viewDeploymentConfig: true,
		},
		queries: [
			{
				key: updateCheck().queryKey,
				data: outdatedUpdateCheck,
			},
		],
	},
	play: async () => {
		const body = within(document.body);
		const snackbar = await body.findByTestId("update-check-snackbar");

		await waitFor(() => {
			expect(snackbar).toBeVisible();
		});
	},
};

export const SkipLinkPrecedesNavigation: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const skipToContentLink = await canvas.findByRole("link", {
			name: skipLinkName,
		});
		const [navigation] = await canvas.findAllByRole("navigation");

		await waitFor(() => {
			expect(document.getElementById("main-content")).not.toBeNull();
		});
		const mainContent = getMainContent();

		expect(skipToContentLink).toHaveTextContent(skipLinkName);
		expect(skipToContentLink).toHaveAttribute("href", "#main-content");
		expect(mainContent).toHaveAttribute("tabindex", "-1");
		expect(
			skipToContentLink.compareDocumentPosition(navigation) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
	},
};

export const SkipLinkMovesFocusToMainContent: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const skipToContentLink = await canvas.findByRole("link", {
			name: skipLinkName,
		});

		await waitFor(() => {
			expect(document.getElementById("main-content")).not.toBeNull();
		});
		const mainContent = getMainContent();

		await user.click(skipToContentLink);

		await waitFor(() => {
			expect(mainContent).toHaveFocus();
		});
	},
};
