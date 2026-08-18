import type { StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { HEALTH_QUERY_KEY } from "#/api/queries/debug";
import type { HealthcheckReport } from "#/api/typesGenerated";
import { MockHealth } from "#/testHelpers/entities";
import { generateMeta } from "./storybook";
import UsagePublishingPage from "./UsagePublishingPage";

const meta = {
	title: "pages/Health/UsagePublishing",
	...generateMeta({
		path: "/health/usage-publishing",
		element: <UsagePublishingPage />,
	}),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

const settingsWithEnabledPublishing: HealthcheckReport = {
	...MockHealth,
	usage_publishing: {
		...MockHealth.usage_publishing,
		publishing_enabled: true,
		last_published_at: "2023-10-12T12:00:00.000000000Z",
	},
};

export const PublishingEnabled: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{
				key: HEALTH_QUERY_KEY,
				data: settingsWithEnabledPublishing,
			},
		],
	},
};

const settingsWithFailure: HealthcheckReport = {
	...MockHealth,
	severity: "warning",
	usage_publishing: {
		...MockHealth.usage_publishing,
		severity: "warning",
		publishing_enabled: true,
		failing_since: "2023-10-11T12:00:00.000000000Z",
		warnings: [
			{
				code: "EUP01",
				message:
					"usage events have failed to publish to Coder's servers since 2023-10-11 12:00:00 UTC",
			},
		],
	},
};

export const PublishingFailing: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{
				key: HEALTH_QUERY_KEY,
				data: settingsWithFailure,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(/usage events have failed to publish/),
		).toBeVisible();
		await expect(canvas.getByText("Failing since")).toBeVisible();
	},
};

// During a rolling upgrade an older replica may serve a health report
// without the usage_publishing field. The section must disappear from the
// nav instead of crashing the page.
const { usage_publishing: _, ...reportFromOlderReplica } = MockHealth;

export const HiddenOnOlderReplica: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{
				key: HEALTH_QUERY_KEY,
				data: reportFromOlderReplica,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("link", { name: /database/i }),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: /usage publishing/i }),
		).not.toBeInTheDocument();
	},
};

export { Example as UsagePublishing };
