import type {
	DeploymentValues,
	Experiments,
	FeatureName,
	Organization,
	SerpentOption,
	User,
} from "api/typesGenerated";
import type { Permissions } from "contexts/auth/permissions";
import type { QueryKey } from "react-query";

declare module "@storybook/react" {
	type WebSocketEvent =
		| { event: "message"; data: string }
		| { event: "error" | "close" };
	interface Parameters {
		features?: FeatureName[];
		experiments?: Experiments;
		showOrganizations?: boolean;
		organizations?: Organization[];
		queries?: { key: QueryKey; data: unknown; isError?: boolean }[];
		webSocket?: WebSocketEvent[];
		user?: User;
		permissions?: Partial<Permissions>;
		deploymentValues?: DeploymentValues;
		deploymentOptions?: SerpentOption[];
	}
}
