import type { StoryObj } from "@storybook/react-vite";
import {
	expect,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { API } from "#/api/api";
import {
	HEALTH_QUERY_KEY,
	HEALTH_QUERY_SETTINGS_KEY,
} from "#/api/queries/debug";
import type { HealthcheckReport } from "#/api/typesGenerated";
import {
	MockHealth,
	MockHealthSettings,
	mockApiError,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
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

// MockHealth reports publishing disabled, the default for deployments
// without a publishing-enabled license, including air-gapped ones.
const Example: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Publishing enabled")).toBeVisible();
		await expect(canvas.getByText("No")).toBeVisible();
		await expect(canvas.getByText("Last published")).toBeVisible();
		await expect(
			canvas.getByText("No recent successful publish"),
		).toBeVisible();
		await expect(canvas.queryByText("Failing since")).not.toBeInTheDocument();
		await expect(
			canvas.queryByText(/usage events have failed to publish/),
		).not.toBeInTheDocument();
	},
};

// Relative to now so the rendered relative-time string is deterministic.
const lastPublishedAt = new Date(
	Date.now() - 2 * 24 * 60 * 60 * 1000,
).toISOString();

const settingsWithEnabledPublishing: HealthcheckReport = {
	...MockHealth,
	usage_publishing: {
		...MockHealth.usage_publishing,
		publishing_enabled: true,
		last_published_at: lastPublishedAt,
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Publishing enabled")).toBeVisible();
		await expect(canvas.getByText("Yes")).toBeVisible();
		await expect(canvas.getByText("Last published")).toBeVisible();
		await expect(canvas.getByText("2 days ago")).toBeVisible();
		await expect(
			canvas.queryByText("No recent successful publish"),
		).not.toBeInTheDocument();
		await expect(canvas.queryByText("Failing since")).not.toBeInTheDocument();
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

// When the status query fails server-side, last_published_at is unset
// because its value is unknown, not because there was no recent success.
// The page must render an unknown value instead of a factual-looking "no
// recent successful publish".
const settingsWithUnavailableStatus: HealthcheckReport = {
	...MockHealth,
	severity: "warning",
	usage_publishing: {
		...MockHealth.usage_publishing,
		severity: "warning",
		publishing_enabled: true,
		status_unavailable: true,
		warnings: [
			{
				code: "EUNKNOWN",
				message:
					"unable to determine usage publishing status; check the coderd logs",
			},
		],
	},
};

export const StatusUnavailable: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{
				key: HEALTH_QUERY_KEY,
				data: settingsWithUnavailableStatus,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(/unable to determine usage publishing status/),
		).toBeVisible();
		await expect(canvas.getByText("Unknown")).toBeVisible();
		await expect(
			canvas.queryByText("No recent successful publish"),
		).not.toBeInTheDocument();
	},
};

// During a rolling upgrade an older replica may reject the UsagePublishing
// section value in the settings mutation. The failure must surface as an
// error toast instead of an unhandled rejection.
export const MuteFailsOnOlderReplica: Story = {
	decorators: [withToaster],
	beforeEach: () => {
		spyOn(API, "updateHealthSettings").mockRejectedValue(
			mockApiError({
				message: "Failed to validate health settings.",
			}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /mute warnings/i }),
		);
		await waitFor(() => {
			expect(
				screen.getByText("Failed to validate health settings."),
			).toBeInTheDocument();
		});
	},
};

// Unmuting goes through its own mutation and catch block; an older
// replica's rejection must surface as an error toast there too, not as an
// unhandled rejection.
export const UnmuteFailsOnOlderReplica: Story = {
	decorators: [withToaster],
	parameters: {
		queries: [
			...meta.parameters.queries,
			{
				key: HEALTH_QUERY_SETTINGS_KEY,
				data: {
					...MockHealthSettings,
					dismissed_healthchecks: ["UsagePublishing"],
				},
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "updateHealthSettings").mockRejectedValue(
			mockApiError({
				message: "Failed to validate health settings.",
			}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /unmute warnings/i }),
		);
		await waitFor(() => {
			expect(
				screen.getByText("Failed to validate health settings."),
			).toBeInTheDocument();
		});
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
