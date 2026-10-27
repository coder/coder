import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import { templateByNameKey } from "#/api/queries/templates";
import type { TemplateResourceRun } from "#/api/typesGenerated";
import {
	MockTemplate,
	MockTemplateVersion,
	mockApiError,
} from "#/testHelpers/entities";
import { pixelWithPhone } from "#/testHelpers/pixel";
import { withDashboardProvider } from "#/testHelpers/storybook";
import {
	createMockTemplateResourceArchive,
	MockTemplateResourceLog,
	MockTemplateResourceRevision,
	MockTemplateResourceRun,
	MockTemplateResources,
} from "#/testHelpers/templateResources";
import TemplateFilesEditorPage from "./TemplateFilesEditorPage";

const mockPlan: TemplateResourceRun = {
	...MockTemplateResourceRun,
	action: "plan",
};
const mockPlannedResources = {
	...MockTemplateResources,
	applied_revision_id: null,
	runs: [mockPlan],
};
const meta = {
	title: "pages/TemplateFilesEditorPage",
	component: TemplateFilesEditorPage,
	decorators: [withDashboardProvider],
	beforeEach: async () => {
		const mocks = [
			spyOn(API, "getFile").mockResolvedValue(
				await createMockTemplateResourceArchive(),
			),
			spyOn(API, "getTemplateResources").mockResolvedValue({
				...MockTemplateResources,
				runs: [],
			}),
			spyOn(API, "getTemplateResourceRunLogs").mockResolvedValue([
				MockTemplateResourceLog,
			]),
		];
		return () => {
			for (const mock of mocks) mock.mockRestore();
		};
	},
	parameters: {
		layout: "fullscreen",
		pixel: { matrix: pixelWithPhone },
		experiments: ["template-resources"],
		queries: [
			{
				key: templateByNameKey("default", MockTemplate.name),
				data: MockTemplate,
			},
			{
				key: getAuthorizationKey({
					checks: {
						canUpdateTemplate: {
							object: {
								resource_type: "template",
								resource_id: MockTemplate.id,
							},
							action: "update",
						},
					},
				}),
				data: { canUpdateTemplate: true },
			},
		],
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					template: MockTemplate.name,
					version: MockTemplateVersion.name,
				},
				searchParams: { scope: "template", path: "main.tf" },
			},
			routing: { path: "/templates/:template/versions/:version/edit" },
		}),
	},
} satisfies Meta<typeof TemplateFilesEditorPage>;
export default meta;
type Story = StoryObj<typeof meta>;

export const SharedScope: Story = {};
export const Loading: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateByName").mockImplementation(
			() => new Promise(() => {}),
		);
		return () => mock.mockRestore();
	},
};
export const LoadError: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateByName").mockRejectedValue(
			mockApiError({ message: "Template unavailable." }),
		);
		return () => mock.mockRestore();
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("button", { name: "Retry editor" });
	},
};
export const PermissionDenied: Story = {
	parameters: {
		queries: [
			{
				key: templateByNameKey("default", MockTemplate.name),
				data: MockTemplate,
			},
			{
				key: getAuthorizationKey({
					checks: {
						canUpdateTemplate: {
							object: {
								resource_type: "template",
								resource_id: MockTemplate.id,
							},
							action: "update",
						},
					},
				}),
				data: { canUpdateTemplate: false },
			},
		],
	},
};
export const ArchiveError: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFile").mockRejectedValue(
			mockApiError({ message: "Source archive unavailable." }),
		);
		return () => mock.mockRestore();
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("button", { name: "Retry files" });
	},
};
export const Unconfigured: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateResources").mockResolvedValue({
			...MockTemplateResources,
			status: "unconfigured",
			current_revision_id: null,
			applied_revision_id: null,
			revisions: [],
			runs: [],
		});
		return () => mock.mockRestore();
	},
};
export const BuildSucceeded: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateResources").mockResolvedValue(
			mockPlannedResources,
		);
		return () => mock.mockRestore();
	},
};
export const Published: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateResources").mockResolvedValue(
			MockTemplateResources,
		);
		return () => mock.mockRestore();
	},
};

export const BuildRunning: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateResources").mockResolvedValue({
			...mockPlannedResources,
			runs: [{ ...mockPlan, job: { ...mockPlan.job, status: "running" } }],
		});
		return () => mock.mockRestore();
	},
};
export const BuildFailed: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateResources").mockResolvedValue({
			...mockPlannedResources,
			runs: [
				{
					...mockPlan,
					job: {
						...mockPlan.job,
						status: "failed",
						error:
							"modules/coder-network-policy/main.tf: Missing required argument.",
					},
				},
			],
		});
		return () => mock.mockRestore();
	},
};
export const BuildSaveError: Story = {
	beforeEach: () => {
		const upload = spyOn(API, "uploadFile").mockResolvedValue({
			hash: MockTemplateResourceRevision.file_id,
		});
		const save = spyOn(API, "createTemplateResourceRevision").mockRejectedValue(
			mockApiError({ message: "Could not save archive. Try Build again." }),
		);
		return () => {
			upload.mockRestore();
			save.mockRestore();
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(await canvas.findByRole("button", { name: "Build" }));
		await canvas.findByRole("alert");
	},
};
export const OlderDestroyRun: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateResources").mockResolvedValue({
			...MockTemplateResources,
			status: "destroyed",
			runs: [
				{
					...MockTemplateResourceRun,
					action: "destroy",
					revision_id: "older-revision",
				},
			],
		});
		const logs = spyOn(API, "getTemplateResourceRunLogs").mockResolvedValue([
			{
				...MockTemplateResourceLog,
				output: "Destroy complete! Resources: 1 destroyed.",
			},
		]);
		return () => {
			mock.mockRestore();
			logs.mockRestore();
		};
	},
};

export const ResizedEditor: Story = {
	play: async ({ canvasElement }) => {
		const button = await within(canvasElement).findByRole("button", {
			name: "Increase editor height",
		});
		button.focus();
		await userEvent.keyboard("{Enter}{Enter}");
	},
};
export const CreateModuleFile: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("button", { name: "Create File" }),
		);
	},
};
