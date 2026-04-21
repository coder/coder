import type { Meta, StoryObj } from "@storybook/react-vite";
import { type FC, useMemo, useState } from "react";
import { MemoryRouter, Outlet, Route, Routes } from "react-router";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { AgentPageHeader } from "./AgentPageHeader";
import { ChimeButton } from "./ChimeButton";
import { WebPushButton } from "./WebPushButton";

type MediaChangeListener = (event: MediaQueryListEvent) => void;

const createMatchMediaController = (initialDesktop: boolean) => {
	let desktop = initialDesktop;
	const listeners = new Set<MediaChangeListener>();
	const eventListenerWrappers = new Map<
		EventListenerOrEventListenerObject,
		MediaChangeListener
	>();

	const getWrappedEventListener = (
		listener: EventListenerOrEventListenerObject | null,
	): MediaChangeListener | undefined => {
		if (!listener) {
			return undefined;
		}

		const existing = eventListenerWrappers.get(listener);
		if (existing) {
			return existing;
		}

		const wrapped: MediaChangeListener = (event) => {
			if (typeof listener === "function") {
				listener(event);
				return;
			}
			listener.handleEvent(event);
		};

		eventListenerWrappers.set(listener, wrapped);
		return wrapped;
	};

	const dispatch = (): void => {
		const event = {
			matches: desktop,
			media: "(min-width: 768px)",
		} as MediaQueryListEvent;
		for (const listener of listeners) {
			listener(event);
		}
	};

	const matchMedia = ((query: string): MediaQueryList => {
		const isDesktopQuery = query === "(min-width: 768px)";
		return {
			matches: isDesktopQuery ? desktop : false,
			media: query,
			onchange: null,
			addEventListener: (
				_type: string,
				listener: EventListenerOrEventListenerObject | null,
			) => {
				if (isDesktopQuery) {
					const wrapped = getWrappedEventListener(listener);
					if (wrapped) {
						listeners.add(wrapped);
					}
				}
			},
			removeEventListener: (
				_type: string,
				listener: EventListenerOrEventListenerObject | null,
			) => {
				if (isDesktopQuery) {
					const wrapped = getWrappedEventListener(listener);
					if (wrapped) {
						listeners.delete(wrapped);
					}
					if (listener) {
						eventListenerWrappers.delete(listener);
					}
				}
			},
			dispatchEvent: () => true,
			addListener: (listener: MediaChangeListener) => {
				if (isDesktopQuery) {
					listeners.add(listener);
				}
			},
			removeListener: (listener: MediaChangeListener) => {
				if (isDesktopQuery) {
					listeners.delete(listener);
				}
			},
		};
	}) as typeof window.matchMedia;

	return {
		matchMedia,
		setDesktop: (value: boolean) => {
			desktop = value;
			dispatch();
		},
	};
};

let setDesktopViewport: ((desktop: boolean) => void) | undefined;

const HeaderStateHarness: FC = () => {
	const [chimeEnabled, setChimeEnabled] = useState(true);
	const [webpushSubscribed, setWebpushSubscribed] = useState(false);
	const [webpushLoading, setWebpushLoading] = useState(false);

	const webPush = useMemo(
		() => ({
			enabled: true,
			subscribed: webpushSubscribed,
			loading: webpushLoading,
			subscribe: async () => {
				setWebpushLoading(true);
				await Promise.resolve();
				setWebpushSubscribed(true);
				setWebpushLoading(false);
			},
			unsubscribe: async () => {
				setWebpushLoading(true);
				await Promise.resolve();
				setWebpushSubscribed(false);
				setWebpushLoading(false);
			},
		}),
		[webpushLoading, webpushSubscribed],
	);

	const handleNotificationToggle = async () => {
		if (webpushSubscribed) {
			await webPush.unsubscribe();
		} else {
			await webPush.subscribe();
		}
	};

	return (
		<MemoryRouter initialEntries={["/agents"]}>
			<Routes>
				<Route
					element={
						<Outlet
							context={{
								isSidebarCollapsed: false,
								onExpandSidebar: () => undefined,
							}}
						/>
					}
				>
					<Route
						path="/agents"
						element={
							<AgentPageHeader
								chimeEnabled={chimeEnabled}
								onToggleChime={() => setChimeEnabled((enabled) => !enabled)}
								webPush={webPush}
								onToggleNotifications={handleNotificationToggle}
							>
								<ChimeButton
									enabled={chimeEnabled}
									onToggle={() => setChimeEnabled((enabled) => !enabled)}
								/>
								<WebPushButton
									webPush={webPush}
									onToggle={handleNotificationToggle}
								/>
							</AgentPageHeader>
						}
					/>
				</Route>
			</Routes>
		</MemoryRouter>
	);
};

const meta: Meta<typeof AgentPageHeader> = {
	title: "pages/AgentsPage/AgentPageHeader",
	component: AgentPageHeader,
	decorators: [withDashboardProvider],
	beforeEach: () => {
		const originalMatchMedia = window.matchMedia;
		const controller = createMatchMediaController(true);
		window.matchMedia = controller.matchMedia;
		setDesktopViewport = controller.setDesktop;

		return () => {
			window.matchMedia = originalMatchMedia;
			setDesktopViewport = undefined;
		};
	},
};

export default meta;
type Story = StoryObj<typeof AgentPageHeader>;

export const ToggleStateStaysInSyncAcrossBreakpoints: Story = {
	render: () => <HeaderStateHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		const desktopSoundButton = await canvas.findByRole("button", {
			name: "Mute completion chime",
		});
		await userEvent.click(desktopSoundButton);
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: "Enable completion chime" }),
			).toBeVisible();
		});

		const desktopNotificationButton = canvas.getByRole("button", {
			name: "Enable notifications",
		});
		await userEvent.click(desktopNotificationButton);
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: "Disable notifications" }),
			).toBeVisible();
		});

		setDesktopViewport?.(false);
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: "More options" }),
			).toBeVisible();
		});

		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		expect(body.getByRole("menuitem", { name: "Turn sound on" })).toBeVisible();
		expect(
			body.getByRole("menuitem", { name: "Turn notifications off" }),
		).toBeVisible();

		await userEvent.click(
			body.getByRole("menuitem", { name: "Turn notifications off" }),
		);

		setDesktopViewport?.(true);
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: "Enable notifications" }),
			).toBeVisible();
		});
	},
};
