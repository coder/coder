import type { StoryObj } from "@storybook/react-vite";
import { HEALTH_QUERY_KEY } from "#/api/queries/debug";
import type { HealthcheckReport } from "#/api/typesGenerated";
import { MockHealth } from "#/testHelpers/entities";
import PubsubPage from "./PubsubPage";
import { generateMeta } from "./storybook";

const meta = {
	title: "pages/Health/Pubsub",
	...generateMeta({
		path: "/health/pubsub",
		element: <PubsubPage />,
	}),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

const healthWithWarnings: HealthcheckReport = {
	...MockHealth,
	severity: "warning",
	pubsub: {
		...MockHealth.pubsub,
		backend: "nats",
		severity: "warning",
		warnings: [
			{
				code: "EPS02",
				message: "embedded NATS server is not running",
			},
			{
				code: "EPS01",
				message: "NATS publish connection 0 is RECONNECTING",
			},
			{
				code: "EPS01",
				message: "NATS subscribe connection 0 is RECONNECTING",
			},
		],
	},
};

export const WithWarnings: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{ key: HEALTH_QUERY_KEY, data: healthWithWarnings },
		],
	},
};

const healthWithError: HealthcheckReport = {
	...MockHealth,
	severity: "error",
	pubsub: {
		...MockHealth.pubsub,
		severity: "error",
		error: "EUNKNOWN: pubsub report panic: runtime error",
	},
};

export const WithError: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{ key: HEALTH_QUERY_KEY, data: healthWithError },
		],
	},
};

export { Example as Pubsub };
