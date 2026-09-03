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
	// Each story gets its own persisted accordion state, seeded from the
	// `openSections` parameter (or cleared), so interactions in one story
	// cannot leak into another.
	decorators: [
		(Story, { args, parameters }) => {
			const key =
				args.openSectionsStorageKey ?? "user-settings-sidebar-open-sections";
			const seed = parameters.openSections as string[] | undefined;
			if (seed) {
				localStorage.setItem(key, JSON.stringify(seed));
			} else {
				localStorage.removeItem(key);
			}
			// Stories that mount a real CollapsibleSidebar supply the header
			// through its header slot instead.
			const usesRealSidebar = Boolean(parameters.realSidebar);
			return (
				<div className="w-60">
					{!usesRealSidebar && (
						<UserSettingsSidebarHeader user={MockUserOwner} />
					)}
					<Story />
					<LocationProbe />
				</div>
			);
		},
	],
	parameters: { reactRouter: routing("/settings/account") },
	args: {
		showSchedulePage: true,
		showOAuth2Page: true,
	},
};

export default meta;
type Story = StoryObj<typeof UserSettingsSidebarView>;

/** First load with no persisted state: General open, Account active. */
export const Default: Story = {
	args: { openSectionsStorageKey: "story-user-default" },
};

export const Collapsed: Story = {
	args: { openSectionsStorageKey: "story-user-collapsed" },
	decorators: [
		(Story) => (
			<SidebarContext.Provider
				value={{ collapsed: true, expand: () => {}, toggle: () => {} }}
			>
				<Story />
			</SidebarContext.Provider>
		),
	],
};

export const GatesOff: Story = {
	args: {
		openSectionsStorageKey: "story-user-gates-off",
		showSchedulePage: false,
		showOAuth2Page: false,
	},
};

export const SecurityActive: Story = {
	args: { openSectionsStorageKey: "story-user-security" },
	parameters: {
		openSections: [],
		reactRouter: routing("/settings/security"),
	},
};

export const HeaderClickDoesNotNavigate: Story = {
	args: { openSectionsStorageKey: "story-user-header-click" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "General" }));
		await waitFor(() =>
			expect(canvas.queryByRole("link", { name: "Account" })).toBeNull(),
		);
		expect(canvas.getByTestId("location")).toHaveTextContent(
			"/settings/account",
		);
	},
};

// A short viewport with the header pinned: only the nav list scrolls.
export const TallListScrolls: Story = {
	args: { openSectionsStorageKey: "story-user-tall" },
	decorators: [
		(Story) => {
			localStorage.setItem("story-user-tall-width", "expanded");
			return (
				<CollapsibleSidebar
					storageKey="story-user-tall-width"
					header={<UserSettingsSidebarHeader user={MockUserOwner} />}
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: {
		realSidebar: true,
		openSections: ["general"],
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
		const header = canvas.getByText(MockUserOwner.email);
		const headerTop = header.getBoundingClientRect().top;

		await waitFor(() => {
			expect(scrollArea.scrollHeight).toBeGreaterThan(scrollArea.clientHeight);
		});
		expect(document.documentElement.scrollHeight).toBeLessThanOrEqual(
			window.innerHeight,
		);

		scrollArea.scrollTop = scrollArea.scrollHeight;
		await waitFor(() => expect(scrollArea.scrollTop).toBeGreaterThan(0));
		expect(header.getBoundingClientRect().top).toBe(headerTop);
		expect(header).toBeVisible();
	},
};

// Measures header and row geometry against the admin sidebar spec.
export const LayoutMetrics: Story = {
	args: { openSectionsStorageKey: "story-user-metrics" },
	decorators: [
		(Story) => {
			localStorage.setItem("story-user-metrics-width", "expanded");
			return (
				<CollapsibleSidebar
					storageKey="story-user-metrics-width"
					header={<UserSettingsSidebarHeader user={MockUserOwner} />}
				>
					<Story />
				</CollapsibleSidebar>
			);
		},
	],
	parameters: { realSidebar: true, openSections: ["general"] },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sidebar = canvasElement.querySelector("[data-sidebar-container]");
		if (!(sidebar instanceof HTMLElement)) {
			throw new Error("sidebar container not rendered");
		}
		const edge = sidebar.getBoundingClientRect();
		const rect = (element: Element) => element.getBoundingClientRect();

		const avatar = canvas.getByText(MockUserOwner.email).parentElement
			?.previousElementSibling;
		const general = canvas.getByRole("button", { name: "General" });
		const generalIcon = general.querySelector("svg");
		const generalLabel = canvas.getByText("General");
		const account = canvas.getByRole("link", { name: "Account" });
		const appearance = canvas.getByRole("link", { name: "Appearance" });
		const line = account.parentElement;
		const security = canvas.getByRole("link", { name: "Security" });
		const securityIcon = security.querySelector("svg");
		if (!avatar || !generalIcon || !line || !securityIcon) {
			throw new Error("sidebar rows not rendered");
		}

		const metrics = {
			avatarSize: rect(avatar).width,
			avatarTop: rect(avatar).top - edge.top,
			generalRowHeight: rect(general).height,
			generalIconLeft: rect(generalIcon).left - edge.left,
			leafHeight: rect(account).height,
			leafGap: rect(appearance).top - rect(account).bottom,
			lineAtLabelEdge: rect(line).left - rect(generalLabel).left,
			leafTextFromLine:
				rect(account).left + 8 - (rect(line).left + line.clientLeft),
			flatRowHeight: rect(security).height,
			flatIconLeft: rect(securityIcon).left - edge.left,
		};

		expect(metrics.avatarSize).toBe(40);
		expect(metrics.avatarTop).toBe(12);
		expect(metrics.generalRowHeight).toBe(40);
		expect(metrics.generalIconLeft).toBe(16);
		expect(metrics.leafHeight).toBe(32);
		expect(metrics.leafGap).toBe(4);
		expect(metrics.lineAtLabelEdge).toBe(0);
		expect(metrics.leafTextFromLine).toBe(20);
		expect(metrics.flatRowHeight).toBe(40);
		expect(metrics.flatIconLeft).toBe(16);
	},
};
