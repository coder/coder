import { chromatic } from "testHelpers/chromatic";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockEntitlements,
	MockExperiments,
	MockHealth,
	MockHealthSettings,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import type { Meta } from "@storybook/react-vite";
import { HEALTH_QUERY_KEY, HEALTH_QUERY_SETTINGS_KEY } from "api/queries/debug";
import {
	type RouteDefinition,
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { HealthLayout } from "./HealthLayout";

type MetaOptions = {
	element: RouteDefinition;
	path: string;
	params?: Record<string, string>;
};

export const generateMeta = ({ element, path, params }: MetaOptions) => {
	return {
		component: HealthLayout,
		parameters: {
			chromatic,
			layout: "fullscreen",
			reactRouter: reactRouterParameters({
				location: { pathParams: params },
				routing: reactRouterOutlet({ path }, element),
			}),
			queries: [
				{ key: HEALTH_QUERY_KEY, data: MockHealth },
				{ key: HEALTH_QUERY_SETTINGS_KEY, data: MockHealthSettings },
				{ key: ["buildInfo"], data: MockBuildInfo },
				{ key: ["entitlements"], data: MockEntitlements },
				{ key: ["experiments"], data: MockExperiments },
				{ key: ["appearance"], data: MockAppearanceConfig },
			],
			decorators: [withDashboardProvider],
		},
	} satisfies Meta;
};
