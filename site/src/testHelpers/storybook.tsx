import type { StoryContext } from "@storybook/react-vite";
import { withDefaultFeatures } from "api/api";
import { getAuthorizationKey } from "api/queries/authCheck";
import { hasFirstUserKey, meKey } from "api/queries/users";
import type { Entitlements } from "api/typesGenerated";
import { GlobalSnackbar } from "components/GlobalSnackbar/GlobalSnackbar";
import { AuthProvider } from "contexts/auth/AuthProvider";
import {
	getPreferredProxy,
	ProxyContext,
	type ProxyContextValue,
} from "contexts/ProxyContext";
import { DashboardContext } from "modules/dashboard/DashboardProvider";
import { DeploymentConfigContext } from "modules/management/DeploymentConfigProvider";
import { OrganizationSettingsContext } from "modules/management/OrganizationSettingsLayout";
import { permissionChecks } from "modules/permissions";
import type { FC } from "react";
import { useQueryClient } from "react-query";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockDeploymentConfig,
	MockEntitlements,
	MockOrganizationPermissions,
	MockProxyLatencies,
} from "./entities";

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
				features.map((feature) => [
					feature,
					{ enabled: true, entitlement: "entitled" },
				]),
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

export const withWebSocket = (Story: FC, { parameters }: StoryContext) => {
	const events = parameters.webSocket;

	if (!events) {
		console.warn("You forgot to add `parameters.webSocket` to your story");
		return <Story />;
	}

	const listeners = new Map<string, CallbackFn>();
	let callEventsDelay: number;

	window.WebSocket = class WebSocket {
		public readyState = 1;

		addEventListener(type: string, callback: CallbackFn) {
			listeners.set(type, callback);

			// Runs when the last event listener is added
			clearTimeout(callEventsDelay);
			callEventsDelay = window.setTimeout(() => {
				for (const entry of events) {
					const callback = listeners.get(entry.event);

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
	} as unknown as typeof window.WebSocket;

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

export const withGlobalSnackbar = (Story: FC) => (
	<>
		<Story />
		<GlobalSnackbar />
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
