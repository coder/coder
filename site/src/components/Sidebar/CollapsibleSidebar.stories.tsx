import type { Meta, StoryObj } from "@storybook/react-vite";
import { CircleIcon } from "lucide-react";
import type { FC } from "react";
import { Link, useLocation } from "react-router";
import { expect, fireEvent, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { CollapsibleSidebar } from "./CollapsibleSidebar";
import { useSidebarContext } from "./SidebarContext";
import { SidebarGroup } from "./SidebarGroup";
import { SidebarHeader, SidebarHeaderTitle } from "./SidebarHeader";
import { SidebarNavLink } from "./SidebarNavLink";

const STORAGE_KEY = "story-collapsible-sidebar";
const LABEL = "Demo settings";

/** Minimal nav content with a collapsed variant, as the real views have. */
const DemoNav: FC = () => {
	const { collapsed, expand } = useSidebarContext();
	if (collapsed) {
		return (
			<Link
				to="/demo/one"
				onClick={expand}
				aria-label="Demo home"
				className="flex size-10 items-center justify-center rounded-md text-content-secondary hover:bg-surface-secondary"
			>
				<CircleIcon className="size-4" />
			</Link>
		);
	}
	return (
		<SidebarGroup label="Demo">
			<SidebarNavLink href="/demo/one">One</SidebarNavLink>
			<SidebarNavLink href="/demo/two">Two</SidebarNavLink>
		</SidebarGroup>
	);
};

/** Exposes the router location so play functions can assert on it. */
const LocationProbe: FC = () => {
	const { pathname } = useLocation();
	return (
		<p role="status" aria-label="Current location">
			{pathname}
		</p>
	);
};

const meta: Meta<typeof CollapsibleSidebar> = {
	title: "components/Sidebar/CollapsibleSidebar",
	component: CollapsibleSidebar,
	args: {
		label: LABEL,
		storageKey: STORAGE_KEY,
		header: (
			<SidebarHeader>
				<SidebarHeaderTitle>{LABEL}</SidebarHeaderTitle>
			</SidebarHeader>
		),
		children: <DemoNav />,
	},
	// Stories share the page, so seed the preference every time and clear
	// it afterwards.
	beforeEach: () => {
		localStorage.setItem(STORAGE_KEY, "expanded");
		return () => localStorage.removeItem(STORAGE_KEY);
	},
	decorators: [
		(Story) => (
			<div className="flex">
				<div className="relative border-0 border-r border-solid border-border">
					<Story />
				</div>
				<main className="flex-1 p-6 text-sm text-content-secondary">
					<p>Page content</p>
					<LocationProbe />
				</main>
			</div>
		),
	],
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/demo/one" },
			routing: [
				{ path: "/demo/one", useStoryElement: true },
				{ path: "/demo/two", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof CollapsibleSidebar>;

const expectExpanded = async (canvas: ReturnType<typeof within>) => {
	await waitFor(() => {
		expect(canvas.getByRole("link", { name: "One" })).toBeVisible();
		expect(
			canvas.getByRole("button", { name: "Collapse sidebar" }),
		).toBeVisible();
	});
};

const expectCollapsed = async (canvas: ReturnType<typeof within>) => {
	await waitFor(() => {
		expect(canvas.queryByRole("link", { name: "One" })).toBeNull();
		expect(canvas.getByRole("link", { name: "Demo home" })).toBeVisible();
		expect(
			canvas.getByRole("button", { name: "Expand sidebar" }),
		).toBeVisible();
	});
};

/** The header toggle switches variants and persists the choice. */
export const ToggleFromHeader: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expectExpanded(canvas);
		expect(canvas.getByRole("navigation", { name: LABEL })).toBeVisible();

		await userEvent.click(
			canvas.getByRole("button", { name: "Collapse sidebar" }),
		);
		await expectCollapsed(canvas);
		expect(localStorage.getItem(STORAGE_KEY)).toBe("collapsed");

		await userEvent.click(
			canvas.getByRole("button", { name: "Expand sidebar" }),
		);
		await expectExpanded(canvas);
		expect(localStorage.getItem(STORAGE_KEY)).toBe("expanded");
	},
};

/** A persisted collapsed preference wins on a wide viewport. */
export const RestoresCollapsedPreference: Story = {
	beforeEach: () => {
		localStorage.setItem(STORAGE_KEY, "collapsed");
		return () => localStorage.removeItem(STORAGE_KEY);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expectCollapsed(canvas);

		// The collapsed variant's own control re-expands the sidebar.
		await userEvent.click(canvas.getByRole("link", { name: "Demo home" }));
		await expectExpanded(canvas);
	},
};

/**
 * The edge handle is a separator: a press and release inside the dead
 * zone toggles like a click, and a drag snaps in its direction.
 */
export const DragHandle: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const handle = canvas.getByRole("separator", { name: "Resize sidebar" });
		// Synthetic pointer events have no active pointer to capture.
		handle.setPointerCapture = () => {};
		handle.releasePointerCapture = () => {};

		const drag = async (fromX: number, toX: number) => {
			await fireEvent.pointerDown(handle, { clientX: fromX, pointerId: 1 });
			await fireEvent.pointerMove(handle, { clientX: toX, pointerId: 1 });
			await fireEvent.pointerUp(handle, { clientX: toX, pointerId: 1 });
		};

		expect(handle).toHaveAttribute("aria-valuenow", "240");

		// Two pixels of travel is a click, not a drag.
		await drag(240, 242);
		await expectCollapsed(canvas);
		expect(handle).toHaveAttribute("aria-valuenow", "64");

		await drag(64, 66);
		await expectExpanded(canvas);

		// Dragging left past the dead zone collapses.
		await drag(240, 180);
		await expectCollapsed(canvas);
		expect(localStorage.getItem(STORAGE_KEY)).toBe("collapsed");

		// Dragging right from the rail expands.
		await drag(64, 140);
		await expectExpanded(canvas);
		expect(localStorage.getItem(STORAGE_KEY)).toBe("expanded");

		// A drag clamped at the edge it started from keeps the state.
		await drag(240, 400);
		await expectExpanded(canvas);

		// A canceled gesture never toggles, whether or not it moved.
		await fireEvent.pointerDown(handle, { clientX: 240, pointerId: 1 });
		await fireEvent.pointerCancel(handle, { clientX: 240, pointerId: 1 });
		await expectExpanded(canvas);
		await fireEvent.pointerDown(handle, { clientX: 240, pointerId: 1 });
		await fireEvent.pointerMove(handle, { clientX: 120, pointerId: 1 });
		await fireEvent.pointerCancel(handle, { clientX: 120, pointerId: 1 });
		await expectExpanded(canvas);
		expect(localStorage.getItem(STORAGE_KEY)).toBe("expanded");
	},
};

/** The handle is keyboard operable: arrows move between the two widths. */
export const KeyboardHandle: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const handle = canvas.getByRole("separator", { name: "Resize sidebar" });
		handle.focus();

		await userEvent.keyboard("{ArrowLeft}");
		await expectCollapsed(canvas);
		expect(handle).toHaveAttribute("aria-valuenow", "64");

		await userEvent.keyboard("{ArrowRight}");
		await expectExpanded(canvas);
		expect(handle).toHaveAttribute("aria-valuenow", "240");

		await userEvent.keyboard("{Home}");
		await expectCollapsed(canvas);
		await userEvent.keyboard("{End}");
		await expectExpanded(canvas);
	},
};

/**
 * Below lg the sidebar starts collapsed despite the persisted preference,
 * and expanding it there is still a column beside the page, not a drawer.
 */
export const StartsCollapsedBelowLg: Story = {
	parameters: { viewport: { defaultViewport: "ipad" } },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expectCollapsed(canvas);
		// The environmental collapse is not written back to storage.
		expect(localStorage.getItem(STORAGE_KEY)).toBe("expanded");

		await userEvent.click(
			canvas.getByRole("button", { name: "Expand sidebar" }),
		);
		await expectExpanded(canvas);
		expect(within(document.body).queryByRole("dialog")).toBeNull();
	},
};

/**
 * Below md the expanded state is a modal drawer that closes on Escape,
 * outside clicks, and link clicks, returning focus to the rail toggle.
 * The drawer never writes to storage.
 */
export const MobileDrawer: Story = {
	parameters: { viewport: { defaultViewport: "iphone12" } },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);
		await expectCollapsed(canvas);

		const openDrawer = async () => {
			await userEvent.click(
				canvas.getByRole("button", { name: "Expand sidebar" }),
			);
			const drawer = await body.findByRole("dialog", { name: LABEL });
			const inDrawer = within(drawer);
			await waitFor(() => {
				expect(inDrawer.getByRole("link", { name: "One" })).toBeVisible();
			});
			return inDrawer;
		};
		const expectClosed = async () => {
			await waitFor(() => {
				expect(body.queryByRole("dialog")).toBeNull();
				expect(
					canvas.getByRole("button", { name: "Expand sidebar" }),
				).toHaveFocus();
			});
		};

		// Escape.
		let drawer = await openDrawer();
		await waitFor(() => {
			expect(
				drawer.getByRole("button", { name: "Collapse sidebar" }),
			).toHaveFocus();
		});
		await userEvent.keyboard("{Escape}");
		await expectClosed();

		// The drawer's own toggle.
		drawer = await openDrawer();
		await userEvent.click(
			drawer.getByRole("button", { name: "Collapse sidebar" }),
		);
		await expectClosed();

		// Outside click. The page is inert under the drawer, so user-event's
		// pointer-events check is disabled for the press that dismisses it.
		await openDrawer();
		const pageUser = userEvent.setup({ pointerEventsCheck: 0 });
		await pageUser.click(canvas.getByText("Page content"));
		await expectClosed();

		// Following a link.
		drawer = await openDrawer();
		await userEvent.click(drawer.getByRole("link", { name: "Two" }));
		await expectClosed();
		expect(
			canvas.getByRole("status", { name: "Current location" }),
		).toHaveTextContent("/demo/two");

		expect(localStorage.getItem(STORAGE_KEY)).toBe("expanded");
	},
};
