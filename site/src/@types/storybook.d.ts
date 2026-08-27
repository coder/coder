import type {
	DeploymentValues,
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
		| { event: "message"; data: string; delayMs?: number }
		| { event: "open" | "error" | "close"; delayMs?: number };
	interface Parameters {
		features?: (FeatureName | ({ name: FeatureName } & Partial<Feature>))[];
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
