import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useLocation } from "react-router";
import { expect, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	UserSettingsSidebarHeader,
	UserSettingsSidebarView,
} from "./UserSettingsSidebarView";

const STORAGE_KEY = "story-user-settings-sidebar";

/** Renders the current path for assertions. */
const LocationProbe: FC = () => {
	const { pathname } = useLocation();
	return (
		<p role="status" aria-label="Current location">
			{pathname}
		</p>
	);
};

const ROUTES = [
	"/settings/account",
	"/settings/appearance",
	"/settings/notifications",
	"/settings/schedule",
	"/settings/external-auth",
	"/settings/oauth2-provider",
	"/settings/security",
	"/settings/ssh-keys",
	"/settings/tokens",
	"/settings/secrets",
] as const;

const routing = (path: string) =>
	reactRouterParameters({
		location: { path },
		routing: [
			{ path: ROUTES[0], useStoryElement: true },
			...ROUTES.slice(1).map((route) => ({
				path: route,
				useStoryElement: true,
			})),
		],
	});

const meta: Meta<typeof UserSettingsSidebarView> = {
	title: "pages/UserSettingsPage/UserSettingsSidebarView",
	component: UserSettingsSidebarView,
	// Reset the persisted state per story.
	beforeEach: () => {
		localStorage.setItem(STORAGE_KEY, "expanded");
		return () => localStorage.removeItem(STORAGE_KEY);
	},
	decorators: [
		(Story) => (
			<div className="flex">
				<div className="relative border-0 border-r border-solid border-border">
					<CollapsibleSidebar
						label="Your account"
						storageKey={STORAGE_KEY}
						header={<UserSettingsSidebarHeader />}
					>
						<Story />
					</CollapsibleSidebar>
				</div>
				<LocationProbe />
			</div>
		),
	],
	parameters: { reactRouter: routing("/settings/account") },
	args: {
		user: MockUserOwner,
		showSchedulePage: true,
		showOAuth2Page: true,
	},
};

export default meta;
type Story = StoryObj<typeof UserSettingsSidebarView>;

/** Every group expanded, Account information active. */
export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Your account")).toBeVisible();
		expect(canvas.getByText(MockUserOwner.email)).toBeVisible();
		expect(
			canvas.getAllByRole("link").map((link) => link.textContent?.trim()),
		).toEqual([
			"Account information",
			"Appearance",
			"Notifications",
			"Schedule",
			"External authentication",
			"OAuth2 applications",
			"SSH keys",
			"Secrets",
			"Tokens",
			"Security",
		]);
		expect(
			canvas.getByRole("link", { name: "Account information" }),
		).toHaveAttribute("aria-current", "page");
	},
};

/** Collapsed, only the avatar shows; clicking it expands. */
export const Collapsed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Collapse sidebar" }),
		);

		const displayName = MockUserOwner.name || MockUserOwner.username;
		const avatar = canvas.getByRole("link", { name: displayName });
		expect(canvas.getAllByRole("link")).toHaveLength(1);
		expect(avatar).toHaveAttribute("href", "/settings/account");
		expect(canvas.queryByText(MockUserOwner.email)).toBeNull();

		await userEvent.click(avatar);
		expect(canvas.getByText(MockUserOwner.email)).toBeVisible();
		expect(canvas.getByRole("link", { name: "Security" })).toBeVisible();
	},
};

export const GatesOff: Story = {
	args: {
		showSchedulePage: false,
		showOAuth2Page: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByRole("link", { name: "Schedule" })).toBeNull();
		expect(
			canvas.queryByRole("link", { name: "OAuth2 applications" }),
		).toBeNull();
		// The group remains with one link.
		expect(canvas.getByText("Connected accounts")).toBeVisible();
		expect(
			canvas.getByRole("link", { name: "External authentication" }),
		).toBeVisible();
	},
};

export const SecurityActive: Story = {
	parameters: {
		reactRouter: routing("/settings/security"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("link", { name: "Security" })).toHaveAttribute(
			"aria-current",
			"page",
		);
		expect(
			canvas.getByRole("link", { name: "Account information" }),
		).not.toHaveAttribute("aria-current");
	},
};

// Group headings are not controls.
export const GroupHeadingIsStatic: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByRole("button", { name: "General" })).toBeNull();
		await userEvent.click(canvas.getByText("General"));
		expect(
			canvas.getByRole("link", { name: "Account information" }),
		).toBeVisible();
		expect(
			canvas.getByRole("status", { name: "Current location" }),
		).toHaveTextContent("/settings/account");
	},
};

/** Short viewport: the identity block scrolls under the pinned header. */
export const ShortViewport: Story = {
	parameters: {
		viewport: {
			options: {
				shortDesktop: {
					name: "Short desktop",
					styles: { width: "1200px", height: "360px" },
				},
			},
			defaultViewport: "shortDesktop",
		},
	},
};
