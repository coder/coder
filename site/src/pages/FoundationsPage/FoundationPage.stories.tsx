import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { foundationPermissionsKey } from "#/api/queries/foundations";
import { withDashboardProvider } from "#/testHelpers/storybook";
import {
	createMockTemplateResourceArchive,
	MockFoundation,
	MockFoundationPermissions,
	MockFoundationPlanPreview,
	MockFoundationWithOutputs,
	MockTemplateResourceLog,
	MockTemplateResourceRun,
} from "#/testHelpers/templateResources";
import { FoundationContent } from "./FoundationPage";

const mockUnappliedFoundation = {
	...MockFoundation,
	status: "unconfigured",
	resources: [],
	applied_revision_id: null,
	checkpoint_revision_id: null,
	outputs: {},
	output_metadata: {},
	runs: [],
};
const mockDestroyedFoundation = {
	...mockUnappliedFoundation,
	status: "destroyed",
	runs: [{ ...MockTemplateResourceRun, action: "destroy" }],
};
const mockUnavailableInventoryFoundation = {
	...MockFoundation,
	resources_available: false,
	resources: [],
};
const mockNoSourceFoundation = {
	...mockUnappliedFoundation,
	revisions: [],
	current_revision_id: null,
};

const meta = {
	title: "pages/Foundations/Management",
	component: FoundationContent,
	args: { resources: MockFoundation },
	parameters: {
		layout: "fullscreen",
		queries: [
			{
				key: foundationPermissionsKey(
					MockFoundation.organization_id,
					MockFoundation.id,
				),
				data: MockFoundationPermissions,
			},
		],
		reactRouter: reactRouterParameters({
			location: {
				path: `/organizations/${MockFoundation.organization_id}/foundations/${MockFoundation.id}`,
				searchParams: { tab: "files" },
			},
			routing: { path: "/organizations/:organization/foundations/:foundation" },
		}),
	},
	decorators: [
		withDashboardProvider,
		(Story) => (
			<div className="min-h-screen flex flex-col">
				<Story />
			</div>
		),
	],
	beforeEach: async () => {
		const mocks = [
			spyOn(API, "getFile").mockResolvedValue(
				await createMockTemplateResourceArchive(),
			),
			spyOn(API, "getFoundationRunLogs").mockResolvedValue([
				MockTemplateResourceLog,
			]),
		];
		return () => {
			for (const mock of mocks) mock.mockRestore();
		};
	},
} satisfies Meta<typeof FoundationContent>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Standalone: Story = {};
export const Resources: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("link", { name: "Resources" }),
		);
	},
};
export const Versions: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("link", { name: "Versions" }),
		);
	},
};
export const Runs: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("link", { name: "Runs" }),
		);
	},
};
export const Menu: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("button", { name: "Open menu" }),
		);
	},
};
export const Settings: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("button", { name: "Open menu" }),
		);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "Settings" }),
		);
	},
};
export const UnappliedResources: Story = {
	args: { resources: mockUnappliedFoundation },
	play: Resources.play,
};
export const InventoryUnavailable: Story = {
	args: { resources: mockUnavailableInventoryFoundation },
	play: Resources.play,
};
export const NoSource: Story = {
	args: { resources: mockNoSourceFoundation },
};
export const SourceLoading: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFile").mockImplementation(
			() => new Promise(() => {}),
		);
		return () => mock.mockRestore();
	},
};
export const SourceError: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFile").mockRejectedValue(
			new Error("Unable to load the Foundation source archive."),
		);
		return () => mock.mockRestore();
	},
};
export const UnknownState: Story = {
	args: { resources: { ...MockFoundation, status: "unknown" } },
	play: Versions.play,
};
export const Pending: Story = {
	args: {
		resources: {
			...MockFoundation,
			status: "pending",
			runs: [
				{
					...MockTemplateResourceRun,
					job: { ...MockTemplateResourceRun.job, status: "running" },
				},
			],
		},
	},
	play: Versions.play,
};
export const IndependentEditor: Story = { args: { edit: true } };
export const ApplyWarning: Story = {
	args: {
		edit: true,
		resources: {
			...MockFoundation,
			applied_revision_id: null,
			runs: [{ ...MockTemplateResourceRun, action: "plan" }],
		},
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("button", { name: "Apply" }),
		);
	},
};
export const ConsumedOutputWarning: Story = {
	args: {
		resources: {
			...MockFoundation,
			runs: [
				{
					...MockTemplateResourceRun,
					action: "plan",
					preview: MockFoundationPlanPreview,
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("link", { name: "Versions" }),
		);
		await userEvent.click(await canvas.findByRole("button", { name: "Apply" }));
	},
};
export const SensitiveOutputs: Story = {
	args: { resources: MockFoundationWithOutputs },
	play: Resources.play,
};
export const SensitiveOutputValuesRedacted: Story = {
	args: {
		resources: {
			...MockFoundationWithOutputs,
			outputs: {
				...MockFoundationWithOutputs.outputs,
				password: "must-not-be-disclosed",
				credentials: { token: "must-not-be-disclosed" },
				missing_metadata: "must-not-be-disclosed",
			},
		},
	},
	play: Resources.play,
};
export const PublicOutputValueUnavailable: Story = {
	args: { resources: { ...MockFoundationWithOutputs, outputs: {} } },
	play: Resources.play,
};
export const OutputsMobile: Story = {
	args: { resources: MockFoundationWithOutputs },
	globals: { viewport: { value: "iphone12", isRotated: false } },
	play: Resources.play,
};
export const DestructionProtectionDisabled: Story = {
	args: {
		resources: {
			...MockFoundation,
			prevent_destroy_with_workspaces: false,
			consumers: { total: 3, running: 1, stopped: 1, unknown: 1 },
		},
	},
	play: Settings.play,
};
export const DestroyedFoundation: Story = {
	args: { resources: mockDestroyedFoundation },
	play: Versions.play,
};
export const DeleteAfterCleanup: Story = {
	args: { resources: mockDestroyedFoundation },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Open menu" }),
		);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "Settings" }),
		);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Delete Foundation" }),
		);
	},
};
export const FailedInitialApplyCleanup: Story = {
	args: {
		resources: {
			...mockUnappliedFoundation,
			status: "failed",
			checkpoint_revision_id: MockFoundation.checkpoint_revision_id,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("link", { name: "Versions" }),
		);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Destroy" }),
		);
	},
};
export const DestroyKeepsFoundation: Story = {
	args: {
		resources: {
			...MockFoundation,
			prevent_destroy_with_workspaces: false,
			consumers: { total: 2, running: 1, stopped: 1, unknown: 0 },
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("link", { name: "Versions" }),
		);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Destroy" }),
		);
	},
};
export const ReadOnly: Story = {
	parameters: {
		queries: [
			{
				key: foundationPermissionsKey(
					MockFoundation.organization_id,
					MockFoundation.id,
				),
				data: {
					...MockFoundationPermissions,
					canUpdateFoundation: false,
					canUseFoundation: false,
				},
			},
		],
	},
};
