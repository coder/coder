import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { belowLgViewportMediaQuery } from "#/utils/mobile";
import { SettingsNavigation } from "./SettingsNavigation";
import { SettingsNavigationBreadcrumb } from "./SettingsNavigationBreadcrumb";
import { SettingsNavigationSidebar } from "./SettingsNavigationSidebar";
import {
	type SettingsNavigationSection,
	shouldUseOptimisticSelection,
} from "./types";

const accountItem = {
	id: "account",
	label: "Account",
	href: "/account",
	end: true,
};

const appearanceItem = {
	id: "appearance",
	label: "Appearance",
	href: "/appearance",
	end: true,
};

const generalSection: SettingsNavigationSection = {
	id: "general",
	label: "General",
	items: [accountItem, appearanceItem],
};

const credentialsSection: SettingsNavigationSection = {
	id: "credentials",
	label: "Credentials",
	items: [{ id: "tokens", label: "Tokens", href: "/tokens", end: true }],
};

const sections: readonly SettingsNavigationSection[] = [
	generalSection,
	credentialsSection,
];

const setupMatchMedia = (initialMatches: boolean) => {
	let matches = initialMatches;
	const listeners = new Set<EventListenerOrEventListenerObject>();
	vi.stubGlobal(
		"matchMedia",
		vi.fn(
			(query: string): MediaQueryList => ({
				get matches() {
					return query === belowLgViewportMediaQuery ? matches : false;
				},
				media: query,
				onchange: null,
				addEventListener: (
					_type: string,
					listener: EventListenerOrEventListenerObject,
				) => listeners.add(listener),
				removeEventListener: (
					_type: string,
					listener: EventListenerOrEventListenerObject,
				) => listeners.delete(listener),
				addListener: () => undefined,
				removeListener: () => undefined,
				dispatchEvent: () => true,
			}),
		),
	);
	return {
		setMatches: (value: boolean) => {
			matches = value;
			act(() => {
				const event = new Event("change");
				for (const listener of listeners) {
					if (typeof listener === "function") {
						listener(event);
					} else {
						listener.handleEvent(event);
					}
				}
			});
		},
	};
};

const renderWithRouter = (
	element: React.ReactNode,
	initialPath = "/account",
) => {
	const router = createMemoryRouter([{ path: "*", element }], {
		initialEntries: [initialPath],
	});
	render(
		<AppProviders>
			<RouterProvider router={router} />
		</AppProviders>,
	);
	return router;
};

const renderNavigation = (initialPath = "/account") => {
	return renderWithRouter(
		<SettingsNavigation
			title="User settings"
			sections={sections}
			storageKey="test-settings-navigation"
		>
			<div>Settings content</div>
		</SettingsNavigation>,
		initialPath,
	);
};

beforeEach(() => {
	localStorage.clear();
	setupMatchMedia(false);
});

it("only uses optimistic selection for unmodified primary clicks", () => {
	const click = {
		altKey: false,
		button: 0,
		ctrlKey: false,
		metaKey: false,
		shiftKey: false,
	} satisfies Parameters<typeof shouldUseOptimisticSelection>[0];

	expect(shouldUseOptimisticSelection(click)).toBe(true);
	expect(shouldUseOptimisticSelection({ ...click, altKey: true })).toBe(false);
	expect(shouldUseOptimisticSelection({ ...click, button: 1 })).toBe(false);
	expect(shouldUseOptimisticSelection({ ...click, ctrlKey: true })).toBe(false);
	expect(shouldUseOptimisticSelection({ ...click, metaKey: true })).toBe(false);
	expect(shouldUseOptimisticSelection({ ...click, shiftKey: true })).toBe(
		false,
	);
});

it("does not select a sidebar item for a modified click", async () => {
	const user = userEvent.setup();
	const onSelect = vi.fn();
	renderWithRouter(
		<SettingsNavigationSidebar
			title="User settings"
			titleLayoutId="test-title"
			dividerLayoutId="test-divider"
			sections={sections}
			active={{ section: generalSection, item: accountItem }}
			onCollapse={vi.fn()}
			onSelect={onSelect}
			animateControls={false}
			toggleRef={createRef<HTMLButtonElement>()}
		/>,
	);
	const appearanceLink = screen.getByRole("link", { name: "Appearance" });
	appearanceLink.addEventListener("click", (event) => event.preventDefault());

	await user.keyboard("{Control>}");
	await user.click(appearanceLink);
	await user.keyboard("{/Control}");

	expect(onSelect).not.toHaveBeenCalled();
});

it("does not select a breadcrumb item for a modified click", async () => {
	const user = userEvent.setup();
	const onSelect = vi.fn();
	renderWithRouter(
		<SettingsNavigationBreadcrumb
			title="User settings"
			titleLayoutId="test-title"
			dividerLayoutId="test-divider"
			sections={sections}
			active={{ section: generalSection, item: accountItem }}
			onExpand={vi.fn()}
			onSelect={onSelect}
			animateControls={false}
			toggleRef={createRef<HTMLButtonElement>()}
		/>,
	);
	await user.click(screen.getByRole("button", { name: "Account" }));
	const appearanceItem = screen.getByRole("menuitem", { name: "Appearance" });
	appearanceItem.addEventListener("click", (event) => event.preventDefault());

	await user.keyboard("{Control>}");
	await user.click(appearanceItem);
	await user.keyboard("{/Control}");

	expect(onSelect).not.toHaveBeenCalled();
});

it("moves focus to the replacement toggle when the mode changes", async () => {
	const user = userEvent.setup();
	renderNavigation();

	await user.click(screen.getByRole("button", { name: "Collapse navigation" }));
	const expandToggle = screen.getByRole("button", {
		name: "Expand navigation",
	});
	expect(expandToggle).toHaveFocus();

	await user.click(expandToggle);
	expect(
		screen.getByRole("button", { name: "Collapse navigation" }),
	).toHaveFocus();
});

it("locks document scrolling while mounted", () => {
	const router = createMemoryRouter(
		[
			{
				path: "*",
				element: (
					<SettingsNavigation
						title="User settings"
						sections={sections}
						storageKey="test-settings-navigation"
					>
						<div>Settings content</div>
					</SettingsNavigation>
				),
			},
		],
		{ initialEntries: ["/account"] },
	);
	const result = render(
		<AppProviders>
			<RouterProvider router={router} />
		</AppProviders>,
	);

	expect(document.documentElement.dataset.settingsNavigation).toBe("");
	result.unmount();
	expect(document.documentElement.dataset.settingsNavigation).toBeUndefined();
});

it("resets and restores content scroll by history entry", async () => {
	const router = renderNavigation();
	const scroller = screen.getByRole("region", {
		name: "User settings content",
	});

	act(() => {
		scroller.scrollTop = 120;
	});
	await act(() => router.navigate("/appearance"));
	expect(scroller.scrollTop).toBe(0);

	act(() => {
		scroller.scrollTop = 240;
	});
	await act(() => router.navigate("/tokens", { replace: true }));
	expect(scroller.scrollTop).toBe(0);

	act(() => {
		scroller.scrollTop = 360;
	});
	await act(() => router.navigate(-1));
	expect(scroller.scrollTop).toBe(120);

	act(() => {
		scroller.scrollTop = 180;
	});
	await act(() => router.navigate(1));
	expect(scroller.scrollTop).toBe(360);

	await act(() => router.navigate(-1));
	expect(scroller.scrollTop).toBe(180);
});

it("restores content scroll after settings unmount and browser Back", async () => {
	const router = createMemoryRouter(
		[
			{
				path: "/settings/*",
				element: (
					<SettingsNavigation
						title="User settings"
						sections={sections}
						storageKey="test-settings-navigation-remount"
					>
						<div>Settings content</div>
					</SettingsNavigation>
				),
			},
			{ path: "/outside", element: <div>Outside settings</div> },
		],
		{ initialEntries: ["/settings/account"] },
	);
	render(
		<AppProviders>
			<RouterProvider router={router} />
		</AppProviders>,
	);
	const scroller = screen.getByRole("region", {
		name: "User settings content",
	});

	act(() => {
		scroller.scrollTop = 420;
	});
	await act(() => router.navigate("/outside"));
	await act(() => router.navigate(-1));

	expect(
		screen.getByRole("region", { name: "User settings content" }).scrollTop,
	).toBe(420);
});

it("navigates when a sidebar item is selected", async () => {
	const user = userEvent.setup();
	const router = renderNavigation();

	await user.click(screen.getByRole("link", { name: "Appearance" }));

	expect(router.state.location.pathname).toBe("/appearance");
});

it("persists a manual desktop collapse preference", async () => {
	const user = userEvent.setup();
	renderNavigation();

	await user.click(screen.getByRole("button", { name: "Collapse navigation" }));

	expect(localStorage.getItem("test-settings-navigation")).toBe("true");
});

it("resets temporary expansion after leaving and re-entering narrow mode", async () => {
	const media = setupMatchMedia(true);
	const user = userEvent.setup();
	const router = renderNavigation();

	await user.click(screen.getByRole("button", { name: "Expand navigation" }));
	media.setMatches(false);
	media.setMatches(true);
	await user.click(screen.getByRole("button", { name: "Account" }));
	await user.click(screen.getByRole("menuitem", { name: "Tokens" }));

	expect(router.state.location.pathname).toBe("/tokens");
});

it("allows a temporary expansion below the large breakpoint", async () => {
	setupMatchMedia(true);
	const user = userEvent.setup();
	const router = renderNavigation();

	await user.click(screen.getByRole("button", { name: "Expand navigation" }));
	await user.click(screen.getByRole("link", { name: "Tokens" }));

	await waitFor(() => {
		expect(router.state.location.pathname).toBe("/tokens");
	});
	expect(localStorage.getItem("test-settings-navigation")).toBeNull();
});
