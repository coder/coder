import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";

import type { TemplateResourceRun } from "#/api/typesGenerated";
import FoundationPage from "#/pages/FoundationsPage/FoundationPage";
import { mockApiError } from "#/testHelpers/entities";
import { pixelWithPhone } from "#/testHelpers/pixel";
import { withDashboardProvider } from "#/testHelpers/storybook";
import {
	createMockTemplateResourceArchive,
	MockFoundation,
	MockTemplateResourceLog,
	MockTemplateResourceRevision,
	MockTemplateResourceRun,
} from "#/testHelpers/templateResources";

const mockPlan: TemplateResourceRun = {
	...MockTemplateResourceRun,
	action: "plan",
};
const mockPlannedResources = {
	...MockFoundation,
	applied_revision_id: null,
	runs: [mockPlan],
};
const meta = {
	title: "pages/Foundations/FilesEditor",
	component: FoundationPage,
	decorators: [withDashboardProvider],
	beforeEach: async () => {
		const mocks = [
			spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
				Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
			),
			spyOn(API, "getFile").mockResolvedValue(
				await createMockTemplateResourceArchive(),
			),
			spyOn(API, "getFoundation").mockResolvedValue({
				...MockFoundation,
				runs: [],
			}),
			spyOn(API, "getFoundationRunLogs").mockResolvedValue([
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

		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					organization: MockFoundation.organization_id,
					foundation: MockFoundation.id,
				},
				searchParams: { edit: "true", path: "main.tf" },
			},
			routing: { path: "/organizations/:organization/foundations/:foundation" },
		}),
	},
} satisfies Meta<typeof FoundationPage>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Loading: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFoundation").mockImplementation(
			() => new Promise(() => {}),
		);
		return () => mock.mockRestore();
	},
};
export const LoadError: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFoundation").mockRejectedValue(
			mockApiError({ message: "Template unavailable." }),
		);
		return () => mock.mockRestore();
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("button", {
			name: "Retry Foundation",
		});
	},
};
export const PermissionDenied: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "checkAuthorization").mockResolvedValue({
			canReadFoundation: false,
		});
		return () => mock.mockRestore();
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
		const mock = spyOn(API, "getFoundation").mockResolvedValue({
			...MockFoundation,
			status: "unconfigured",
			current_revision_id: null,
			applied_revision_id: null,
			revisions: [],
			runs: [],
		});
		return () => mock.mockRestore();
	},
};
export const PlanSucceeded: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFoundation").mockResolvedValue(
			mockPlannedResources,
		);
		return () => mock.mockRestore();
	},
};
export const Applied: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFoundation").mockResolvedValue(MockFoundation);
		return () => mock.mockRestore();
	},
};

export const PlanRunning: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFoundation").mockResolvedValue({
			...mockPlannedResources,
			runs: [{ ...mockPlan, job: { ...mockPlan.job, status: "running" } }],
		});
		return () => mock.mockRestore();
	},
};
export const PlanFailed: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFoundation").mockResolvedValue({
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
export const SaveError: Story = {
	beforeEach: () => {
		const upload = spyOn(API, "uploadFile").mockResolvedValue({
			hash: MockTemplateResourceRevision.file_id,
		});
		const save = spyOn(API, "createFoundationRevision").mockRejectedValue(
			mockApiError({
				message: "Could not save archive. Try Save files again.",
			}),
		);
		return () => {
			upload.mockRestore();
			save.mockRestore();
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: "Save files" }),
		);
		await canvas.findByRole("alert");
	},
};
export const OlderDestroyRun: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getFoundation").mockResolvedValue({
			...MockFoundation,
			status: "destroyed",
			runs: [
				{
					...MockTemplateResourceRun,
					action: "destroy",
					revision_id: "older-revision",
				},
			],
		});
		const logs = spyOn(API, "getFoundationRunLogs").mockResolvedValue([
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
