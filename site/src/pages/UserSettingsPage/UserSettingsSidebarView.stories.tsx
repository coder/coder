import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useLocation } from "react-router";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { SidebarContext } from "#/components/Sidebar/SidebarContext";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	UserSettingsSidebarHeader,
	UserSettingsSidebarView,
} from "./UserSettingsSidebarView";

/** Exposes the router location so play functions can assert on it. */
const LocationProbe: FC = () => {
	const { pathname } = useLocation();
	return <div data-testid="location">{pathname}</div>;
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
];

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
	decorators: [
		(Story, { parameters }) => {
			// Stories that mount a real CollapsibleSidebar supply the header
			// through its header slot instead.
			const usesRealSidebar = Boolean(parameters.realSidebar);
			return (
				<div className="w-60">
					{!usesRealSidebar && <UserSettingsSidebarHeader />}
					<Story />
					<LocationProbe />
				</div>
			);
		},
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
	},
};

/** The icon rail: the avatar links to the account page. */
export const Collapsed: Story = {
	decorators: [
		(Story) => (
			<SidebarContext.Provider
				value={{ collapsed: true, expand: () => {}, toggle: () => {} }}
			>
				<Story />
			</SidebarContext.Provider>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getAllByRole("link")).toHaveLength(1);
		expect(canvas.getByRole("link")).toHaveAttribute(
			"href",
			"/settings/account",
		);
		expect(canvas.queryByText(MockUserOwner.email)).toBeNull();
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
		// Connected accounts still renders with its single remaining link.
		expect(canvas.getByText("Connected accounts")).toBeVisible();
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
	},
};

// The group headings are labels, not controls: clicking one neither
// navigates nor hides its links.
export const GroupHeadingIsStatic: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByRole("button", { name: "General" })).toBeNull();
		await userEvent.click(canvas.getByText("General"));
		expect(
			canvas.getByRole("link", { name: "Account information" }),
		).toBeVisible();
		expect(canvas.getByTestId("location")).toHaveTextContent(
			"/settings/account",
		);
	},
};

// A short viewport with the header pinned: the identity block scrolls
// away with the list while the title and toggle stay put.
export const TallListScrolls: Story = {
	decorators: [
		(Story) => {
			localStorage.setItem("story-user-tall-width", "expanded");
			return (
				<CollapsibleSidebar
					storageKey="story-user-tall-width"
					header={<UserSettingsSidebarHeader />}
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: {
		realSidebar: true,
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scrollArea = canvas.getByTestId("sidebar-scroll-area");
		const title = canvas.getByText("Your account");
		const email = canvas.getByText(MockUserOwner.email);
		const titleTop = title.getBoundingClientRect().top;
		const emailTop = email.getBoundingClientRect().top;

		await waitFor(() => {
			expect(scrollArea.scrollHeight).toBeGreaterThan(scrollArea.clientHeight);
		});
		expect(document.documentElement.scrollHeight).toBeLessThanOrEqual(
			window.innerHeight,
		);

		scrollArea.scrollTop = scrollArea.scrollHeight;
		await waitFor(() => expect(scrollArea.scrollTop).toBeGreaterThan(0));
		expect(title.getBoundingClientRect().top).toBe(titleTop);
		expect(email.getBoundingClientRect().top).toBeLessThan(emailTop);
	},
};

// Measures header and row geometry against the admin sidebar spec.
export const LayoutMetrics: Story = {
	decorators: [
		(Story) => {
			localStorage.setItem("story-user-metrics-width", "expanded");
			return (
				<CollapsibleSidebar
					storageKey="story-user-metrics-width"
					header={<UserSettingsSidebarHeader />}
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: { realSidebar: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sidebar = canvasElement.querySelector("[data-sidebar-container]");
		if (!(sidebar instanceof HTMLElement)) {
			throw new Error("sidebar container not rendered");
		}
		const edge = sidebar.getBoundingClientRect();
		const rect = (element: Element) => element.getBoundingClientRect();

		const header = canvas.getByTestId("sidebar-panel").firstElementChild;
		const avatar = canvas.getByText(MockUserOwner.email).parentElement
			?.previousElementSibling;
		const generalLabel = canvas.getByText("General");
		const general = generalLabel.parentElement;
		const account = canvas.getByRole("link", { name: "Account information" });
		const appearance = canvas.getByRole("link", { name: "Appearance" });
		const line = account.parentElement;
		if (!header || !avatar || !general || !line) {
			throw new Error("sidebar rows not rendered");
		}

		const metrics = {
			headerHeight: rect(header).height,
			avatarSize: rect(avatar).width,
			avatarLeft: rect(avatar).left - edge.left,
			headingHeight: rect(general).height,
			headingLabelLeft: rect(generalLabel).left - edge.left,
			leafHeight: rect(account).height,
			leafGap: rect(appearance).top - rect(account).bottom,
			headingToLine: rect(line).top - rect(general).bottom,
			lineAtLabelEdge: rect(line).left - rect(generalLabel).left,
			leafTextFromLine:
				rect(account).left + 8 - (rect(line).left + line.clientLeft),
		};

		expect(metrics.headerHeight).toBe(56);
		expect(metrics.avatarSize).toBe(40);
		expect(metrics.avatarLeft).toBe(16);
		expect(metrics.headingHeight).toBe(32);
		expect(metrics.headingLabelLeft).toBe(16);
		expect(metrics.leafHeight).toBe(32);
		expect(metrics.leafGap).toBe(0);
		expect(metrics.headingToLine).toBe(0);
		expect(metrics.lineAtLabelEdge).toBe(0);
		expect(metrics.leafTextFromLine).toBe(12);
	},
};
