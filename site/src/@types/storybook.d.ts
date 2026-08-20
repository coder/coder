import type {
	DeploymentValues,
	Entitlements,
	Experiments,
	Feature,
	FeatureName,
	Organization,
	SerpentOption,
	User,
} from "#/api/typesGenerated";
import type { Permissions } from "#/modules/permissions";
import type { QueryKey } from "react-query";
import type { ReactRouterAddonStoryParameters } from "storybook-addon-remix-react-router";

declare module "@storybook/react-vite" {
	type WebSocketEvent =
		| { event: "message"; data: string }
		| { event: "open" | "error" | "close" };
	interface Parameters {
		features?: (FeatureName | ({ name: FeatureName } & Partial<Feature>))[];
		/** Overrides applied on top of the entitlements built from `features`. */
		entitlements?: Partial<Entitlements>;
		experiments?: Experiments;
		showOrganizations?: boolean;
		organizations?: Organization[];
		queries?: { key: QueryKey; data: unknown; isError?: boolean }[];
		webSocket?: WebSocketEvent[] | Record<string, WebSocketEvent[]>;
		user?: User;
		permissions?: Partial<Permissions>;
		deploymentValues?: DeploymentValues;
		deploymentOptions?: SerpentOption[];
		reactRouter?: ReactRouterAddonStoryParameters;
	}
}
