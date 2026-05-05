import type { Meta, StoryObj } from "@storybook/react-vite";
import { type FC, useMemo, useState } from "react";
import { Outlet } from "react-router";
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
			media: "(min-width: 640px)",
		} as MediaQueryListEvent;
		for (const listener of listeners) {
			listener(event);
		}
	};

	const matchMedia = ((query: string): MediaQueryList => {
		const isDesktopQuery = /\(\s*min-width\s*:\s*640px\s*\)/.test(query);
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
			<WebPushButton webPush={webPush} onToggle={handleNotificationToggle} />
		</AgentPageHeader>
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

		return () => {
			window.matchMedia = originalMatchMedia;
		};
	},
};

export default meta;
type Story = StoryObj<typeof AgentPageHeader>;

export const ToggleStateStaysInSyncAcrossBreakpoints: Story = {
	render: () => <HeaderStateHarness />,
	parameters: {
		reactRouter: {
			location: {
				path: "/agents",
			},
			routing: [
				{
					path: "/",
					element: (
						<Outlet
							context={{
								isSidebarCollapsed: false,
								onExpandSidebar: () => undefined,
							}}
						/>
					),
					children: [{ path: "agents", useStoryElement: true }],
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

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

		await userEvent.click(
			canvas.getByRole("button", { name: "Disable notifications" }),
		);
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: "Enable notifications" }),
			).toBeVisible();
		});
	},
};
