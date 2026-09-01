import type { StoryContext } from "@storybook/react-vite";
import type { FC } from "react";
import { useQueryClient } from "react-query";
import { expect, waitFor } from "storybook/test";
import { withDefaultFeatures } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import { hasFirstUserKey, meKey } from "#/api/queries/users";
import type { Entitlements } from "#/api/typesGenerated";
import { Toaster } from "#/components/Toaster/Toaster";
import { AuthProvider } from "#/contexts/auth/AuthProvider";
import {
	getPreferredProxy,
	ProxyContext,
	type ProxyContextValue,
} from "#/contexts/ProxyContext";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import { DeploymentConfigContext } from "#/modules/management/DeploymentConfigProvider";
import { OrganizationSettingsContext } from "#/modules/management/OrganizationSettingsLayout";
import { permissionChecks } from "#/modules/permissions";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockDeploymentConfig,
	MockEntitlements,
	MockOrganizationPermissions,
	MockProxyLatencies,
} from "./entities";

/**
 * Wait for Radix to finish tearing down a just-closed layer (Select
 * listbox, Popover, and similar). While the layer is open, Radix marks
 * the rest of the page `aria-hidden` and disables pointer events on it,
 * and it undoes both asynchronously after the closing interaction. A
 * pointer interaction or role query issued before that cleanup lands
 * flakes under CPU load, so re-query the next interaction target until
 * it is back in the accessibility tree and accepts pointer input.
 */
export const waitForRadixLayerClose = async (
	getTarget: () => HTMLElement,
): Promise<void> => {
	await waitFor(
		() => {
			// getTarget must re-query by role so an aria-hidden target throws.
			const target = getTarget();
			expect(window.getComputedStyle(target).pointerEvents).not.toBe("none");
		},
		{ timeout: 10_000 },
	);
};

export const withDashboardProvider = (
	Story: FC,
	{ parameters }: StoryContext,
) => {
	const {
		features = [],
		experiments = [],
		showOrganizations = false,
		organizations = [MockDefaultOrganization],
		canViewOrganizationSettings = false,
	} = parameters;

	const entitlements: Entitlements = {
		...MockEntitlements,
		has_license: features.length > 0,
		features: withDefaultFeatures(
			Object.fromEntries(
				features.map((feature) => {
					if (typeof feature === "string") {
						return [feature, { enabled: true, entitlement: "entitled" }];
					}
					const { name, ...values } = feature;
					return [name, { enabled: true, entitlement: "entitled", ...values }];
				}),
			),
		),
	};

	return (
		<DashboardContext.Provider
			value={{
				entitlements,
				experiments,
				appearance: MockAppearanceConfig,
				buildInfo: {
					...MockBuildInfo,
					version: "v0.0.0-test",
				},
				organizations,
				showOrganizations,
				canViewOrganizationSettings,
			}}
		>
			<Story />
		</DashboardContext.Provider>
	);
};

type MessageEvent = Record<"data", string>;
type CallbackFn = (ev?: MessageEvent) => void;

// parameters.webSocket accepts two formats:
//
//   Array — events are delivered to every socket (backward-compatible):
//     webSocket: [{ event: "message", data: "..." }]
//
//   Record keyed by URL substring — events are delivered only to
//   sockets whose URL contains the key:
//     webSocket: {
//       "/api/v2/chats/": [{ event: "message", data: "..." }],
//       "/api/experimental/workspaceagents/": [{ event: "message", data: "..." }],
//     }
export const withWebSocket = (Story: FC, { parameters }: StoryContext) => {
	const param = parameters.webSocket;

	if (!param) {
		console.warn("You forgot to add `parameters.webSocket` to your story");
		return <Story />;
	}

	const isRouted = !Array.isArray(param);
	const broadcastEvents = isRouted ? [] : param;
	const routedEvents = isRouted ? param : {};

	window.WebSocket = class WebSocket {
		public readyState = 1;
		public binaryType = "blob";
		static OPEN = 1;

		#listeners = new Map<string, CallbackFn>();
		#callEventsDelay: number | undefined;
		#url: string;

		constructor(url?: string) {
			this.#url = url ?? "";
		}

		send() {}

		addEventListener(type: string, callback: CallbackFn) {
			this.#listeners.set(type, callback);

			// Determine which events this socket should receive.
			let events = broadcastEvents;
			if (isRouted) {
				const matchingKey = Object.keys(routedEvents).find((key) =>
					this.#url.includes(key),
				);
				events = matchingKey ? routedEvents[matchingKey] : [];
			}

			if (events.length === 0) {
				return;
			}

			// Runs when the last event listener is added
			clearTimeout(this.#callEventsDelay);
			this.#callEventsDelay = window.setTimeout(() => {
				for (const entry of events) {
					const callback = this.#listeners.get(entry.event);

					if (callback) {
						entry.event === "message"
							? callback({ data: entry.data })
							: callback();
					}
				}
			}, 0);
		}

		removeEventListener(_type: string, _callback: CallbackFn) {}

		close() {}
	} as unknown as typeof WebSocket;

	return <Story />;
};

export const withDesktopViewport = (Story: FC) => (
	<div style={{ width: 1200, height: 800 }}>
		<Story />
	</div>
);

export const withAuthProvider = (Story: FC, { parameters }: StoryContext) => {
	if (!parameters.user) {
		throw new Error("You forgot to add `parameters.user` to your story");
	}
	const queryClient = useQueryClient();
	queryClient.setQueryData(meKey, parameters.user);
	queryClient.setQueryData(hasFirstUserKey, true);
	queryClient.setQueryData(
		getAuthorizationKey({ checks: permissionChecks }),
		parameters.permissions ?? {},
	);

	return (
		<AuthProvider>
			<Story />
		</AuthProvider>
	);
};

export const withToaster = (Story: FC) => (
	<>
		<Story />
		<Toaster />
	</>
);

export const withOrganizationSettingsProvider = (Story: FC) => {
	return (
		<OrganizationSettingsContext.Provider
			value={{
				organizations: [MockDefaultOrganization],
				organizationPermissionsByOrganizationId: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
				},
				organization: MockDefaultOrganization,
				organizationPermissions: MockOrganizationPermissions,
			}}
		>
			<DeploymentConfigContext.Provider
				value={{ deploymentConfig: MockDeploymentConfig }}
			>
				<Story />
			</DeploymentConfigContext.Provider>
		</OrganizationSettingsContext.Provider>
	);
};

export const withProxyProvider =
	(value?: Partial<ProxyContextValue>) => (Story: FC) => {
		return (
			<ProxyContext.Provider
				value={{
					latenciesLoaded: true,
					proxyLatencies: MockProxyLatencies,
					proxy: getPreferredProxy([], undefined),
					proxies: [],
					isLoading: false,
					isFetched: true,
					setProxy: () => {
						return;
					},
					clearProxy: () => {
						return;
					},
					refetchProxyLatencies: (): Date => {
						return new Date();
					},
					...value,
				}}
			>
				<Story />
			</ProxyContext.Provider>
		);
	};
