import type { Meta, StoryObj } from "@storybook/react-vite";
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
} satisfies Meta;

export default meta;
type Story = StoryObj;

export const NATS: Story = {};

const postgresHealth: HealthcheckReport = {
	...MockHealth,
	pubsub: { ...MockHealth.pubsub, backend: "postgres" },
};

export const Postgres: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{ key: HEALTH_QUERY_KEY, data: postgresHealth },
		],
	},
};

const disconnectedHealth: HealthcheckReport = {
	...MockHealth,
	healthy: false,
	severity: "error",
	pubsub: {
		...MockHealth.pubsub,
		connected: false,
		severity: "error",
		error: 'pubsub backend "nats" is disconnected',
	},
};

export const Disconnected: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{ key: HEALTH_QUERY_KEY, data: disconnectedHealth },
		],
	},
};

const unsupportedHealth: HealthcheckReport = {
	...MockHealth,
	severity: "warning",
	pubsub: {
		...MockHealth.pubsub,
		backend: "",
		connected: null,
		last_connection_state_change: undefined,
		severity: "warning",
		warnings: [
			{
				code: "EUNKNOWN",
				message: "pubsub backend does not support health reporting",
			},
		],
	},
};

export const Unsupported: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{ key: HEALTH_QUERY_KEY, data: unsupportedHealth },
		],
	},
};
