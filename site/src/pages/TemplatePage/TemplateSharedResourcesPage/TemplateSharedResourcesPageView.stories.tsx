import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, screen, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { templateResourceRunLogsKey } from "#/api/queries/templateResources";
import type { TemplateResources } from "#/api/typesGenerated";
import { mockApiError } from "#/testHelpers/entities";
import { pixelWithPhone } from "#/testHelpers/pixel";
import {
	MockTemplateResourceLog,
	MockTemplateResourceRevision,
	MockTemplateResourceRun,
	MockTemplateResources,
} from "#/testHelpers/templateResources";
import { TemplateSharedResourcesPageView } from "./TemplateSharedResourcesPageView";

const mockFailedSharedResourceLog: typeof MockTemplateResourceLog = {
	...MockTemplateResourceLog,
	log_level: "error",
	output: "Error: provider credentials are invalid.",
};
const mockHistoricalRevision = {
	...MockTemplateResourceRevision,
	id: "70000000-0000-4000-8000-000000000001",
	created_at: "2026-10-20T12:00:00Z",
};
const mockHistoricalRun = {
	...MockTemplateResourceRun,
	id: "60000000-0000-4000-8000-000000000001",
	revision_id: mockHistoricalRevision.id,
	created_at: "2026-10-20T12:01:00Z",
};
const mockHistoricalLog = {
	...MockTemplateResourceLog,
	output:
		"Historical apply complete. Resources: 2 added, 0 changed, 0 destroyed.",
};
const mockFailedRun: typeof MockTemplateResourceRun = {
	...MockTemplateResourceRun,
	id: "90000000-0000-4000-8000-000000000002",
	created_at: "2026-10-21T12:03:00Z",
	job: {
		...MockTemplateResourceRun.job,
		status: "failed",
		error: "Terraform apply failed: provider credentials are invalid.",
	},
};
const mockRunningRun: typeof MockTemplateResourceRun = {
	...MockTemplateResourceRun,
	id: "90000000-0000-4000-8000-000000000003",
	created_at: "2026-10-21T12:03:00Z",
	job: { ...MockTemplateResourceRun.job, status: "running" },
};
const mockDestroyRun: typeof MockTemplateResourceRun = {
	...MockTemplateResourceRun,
	id: "90000000-0000-4000-8000-000000000004",
	created_at: "2026-10-21T12:03:00Z",
	action: "destroy",
};
const mockDestroyLog = {
	...MockTemplateResourceLog,
	output: "Destroy complete! Resources: 1 destroyed.",
};
const mockReapplyRun: typeof MockTemplateResourceRun = {
	...MockTemplateResourceRun,
	id: "90000000-0000-4000-8000-000000000005",
	revision_id: mockHistoricalRevision.id,
	created_at: "2026-10-21T12:03:00Z",
};
const mockUnconfiguredResources: TemplateResources = {
	...MockTemplateResources,
	status: "unconfigured",
	current_revision_id: null,
	applied_revision_id: null,
	revisions: [],
	runs: [],
	outputs: {},
};
const mockSavedResources: TemplateResources = {
	...MockTemplateResources,
	status: "unconfigured",
	applied_revision_id: null,
	runs: [],
	outputs: {},
};
const mockFailedResources: TemplateResources = {
	...MockTemplateResources,
	status: "failed",
	runs: [mockFailedRun, MockTemplateResourceRun],
};
const mockRunningResources: TemplateResources = {
	...MockTemplateResources,
	status: "pending",
	runs: [mockRunningRun, MockTemplateResourceRun],
};
const mockUnknownResources: TemplateResources = {
	...MockTemplateResources,
	status: "unknown",
	runs: [mockFailedRun, MockTemplateResourceRun],
};
const mockDestroyedResources: TemplateResources = {
	...MockTemplateResources,
	status: "destroyed",
	applied_revision_id: null,
	outputs: {},
	runs: [mockDestroyRun, MockTemplateResourceRun],
};
const mockRevisionHistoryResources: TemplateResources = {
	...MockTemplateResources,
	applied_revision_id: mockHistoricalRevision.id,
	revisions: [MockTemplateResourceRevision, mockHistoricalRevision],
	runs: [mockReapplyRun, MockTemplateResourceRun, mockHistoricalRun],
};
const mockRunHistoryResources: TemplateResources = {
	...MockTemplateResources,
	revisions: [MockTemplateResourceRevision, mockHistoricalRevision],
	runs: [MockTemplateResourceRun, mockHistoricalRun],
};

const meta = {
	title: "pages/TemplatePage/TemplateSharedResourcesPage",
	component: TemplateSharedResourcesPageView,
	beforeEach: () => {
		// Polling overrides the Storybook QueryClient's interval default.
		const logs = spyOn(API, "getTemplateResourceRunLogs").mockImplementation(
			async (_templateId, runId) => {
				if (runId === mockFailedRun.id) return [mockFailedSharedResourceLog];
				if (runId === mockDestroyRun.id) return [mockDestroyLog];
				if (runId === mockHistoricalRun.id) return [mockHistoricalLog];
				if (runId === mockRunningRun.id) return [];
				return [MockTemplateResourceLog];
			},
		);
		return () => logs.mockRestore();
	},
	parameters: {
		queries: [
			{
				key: templateResourceRunLogsKey(
					MockTemplateResources.template_id,
					MockTemplateResourceRun.id,
				),
				data: [MockTemplateResourceLog],
			},
		],
	},
	args: {
		resources: MockTemplateResources,
		error: undefined,
		isLoading: false,
		editFilesHref: "/templates/example/edit?scope=template",
		isRunning: false,
		runError: undefined,
		onRetry: fn(),
		onRun: fn(),
	},
} satisfies Meta<typeof TemplateSharedResourcesPageView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Applied: Story = {
	args: { resources: MockTemplateResources },
	parameters: {
		queries: [
			{
				key: templateResourceRunLogsKey(
					MockTemplateResources.template_id,
					MockTemplateResourceRun.id,
				),
				data: [MockTemplateResourceLog],
			},
		],
	},
};
export const Unconfigured: Story = {
	args: { resources: mockUnconfiguredResources },
};
export const SavedBeforeFirstRun: Story = {
	args: { resources: mockSavedResources },
};
export const Loading: Story = {
	args: { resources: undefined, isLoading: true },
};
export const LoadError: Story = {
	args: {
		resources: undefined,
		error: mockApiError({ message: "Unable to load shared resources." }),
	},
};
export const RefetchError: Story = {
	args: {
		error: mockApiError({
			message:
				"Unable to refresh resource state. Showing the last known state.",
		}),
	},
};
export const ApplyError: Story = {
	args: {
		runError: mockApiError({
			message: "No provisioner available to apply this revision.",
		}),
	},
};
export const FailedRun: Story = {
	beforeEach: () => {
		const logs = spyOn(API, "getTemplateResourceRunLogs").mockResolvedValue([
			mockFailedSharedResourceLog,
		]);
		return () => logs.mockRestore();
	},
	args: { resources: mockFailedResources },
	parameters: {
		queries: [
			{
				key: templateResourceRunLogsKey(
					MockTemplateResources.template_id,
					mockFailedRun.id,
				),
				data: [mockFailedSharedResourceLog],
			},
		],
	},
};
export const Running: Story = {
	beforeEach: () => {
		const logs = spyOn(API, "getTemplateResourceRunLogs").mockResolvedValue([]);
		return () => logs.mockRestore();
	},
	args: { resources: mockRunningResources },
	parameters: {
		queries: [
			{
				key: templateResourceRunLogsKey(
					MockTemplateResources.template_id,
					mockRunningRun.id,
				),
				data: [],
			},
		],
	},
};
export const UnknownState: Story = {
	args: { resources: mockUnknownResources },
	parameters: {
		queries: [
			{
				key: templateResourceRunLogsKey(
					MockTemplateResources.template_id,
					mockFailedRun.id,
				),
				data: [mockFailedSharedResourceLog],
			},
		],
	},
};
export const Destroyed: Story = {
	args: { resources: mockDestroyedResources },
	parameters: {
		queries: [
			{
				key: templateResourceRunLogsKey(
					MockTemplateResources.template_id,
					mockDestroyRun.id,
				),
				data: [mockDestroyLog],
			},
		],
	},
};
export const DestroyConfirmation: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Destroy" }),
		);
	},
};
export const DestroyPending: Story = {
	render: function Render(args) {
		const [isRunning, setIsRunning] = useState(false);
		return (
			<TemplateSharedResourcesPageView
				{...args}
				isRunning={isRunning}
				onRun={() => setIsRunning(true)}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Destroy" }),
		);
		await userEvent.click(
			screen.getByRole("button", { name: "Destroy shared resources" }),
		);
	},
};
export const DestroyError: Story = {
	render: function Render(args) {
		const [runError, setRunError] = useState<unknown>();
		return (
			<TemplateSharedResourcesPageView
				{...args}
				runError={runError}
				onRun={() =>
					setRunError(
						mockApiError({
							message:
								"Delete all workspaces before destroying shared resources.",
						}),
					)
				}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Destroy" }),
		);
		await userEvent.click(
			screen.getByRole("button", { name: "Destroy shared resources" }),
		);
	},
};
export const AppliedRevisionChanged: Story = {
	render: function Render(args) {
		const [resources, setResources] = useState(MockTemplateResources);
		return (
			<TemplateSharedResourcesPageView
				{...args}
				resources={resources}
				onRun={() => setResources(mockRevisionHistoryResources)}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Destroy" }),
		);
		await userEvent.click(
			screen.getByRole("button", { name: "Destroy shared resources" }),
		);
	},
};
export const KeyboardFocus: Story = {
	play: async ({ canvasElement }) => {
		(
			await within(canvasElement).findByRole("link", { name: "Edit files" })
		).focus();
	},
};
export const RevisionHistory: Story = {
	args: { resources: mockRevisionHistoryResources },
	parameters: {
		queries: [
			{
				key: templateResourceRunLogsKey(
					MockTemplateResources.template_id,
					mockReapplyRun.id,
				),
				data: [MockTemplateResourceLog],
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("combobox", { name: "Revision history" }),
		);
	},
};
export const HistoricalRun: Story = {
	args: { resources: mockRunHistoryResources },
	beforeEach: () => {
		const logs = spyOn(API, "getTemplateResourceRunLogs").mockImplementation(
			async (_templateId, runId) =>
				runId === mockHistoricalRun.id
					? [mockHistoricalLog]
					: [MockTemplateResourceLog],
		);
		return () => logs.mockRestore();
	},
	parameters: {
		queries: [
			{
				key: templateResourceRunLogsKey(
					MockTemplateResources.template_id,
					MockTemplateResourceRun.id,
				),
				data: [MockTemplateResourceLog],
			},
			{
				key: templateResourceRunLogsKey(
					MockTemplateResources.template_id,
					mockHistoricalRun.id,
				),
				data: [mockHistoricalLog],
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: `View logs for apply ${mockHistoricalRun.id}`,
			}),
		);
	},
};
export const Mobile: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
	parameters: { layout: "fullscreen", pixel: { matrix: pixelWithPhone } },
};
