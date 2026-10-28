import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn } from "storybook/test";
import { API } from "#/api/api";
import { foundationPermissionsKey } from "#/api/queries/foundations";
import { MockTemplate } from "#/testHelpers/entities";
import {
	MockFoundation,
	MockFoundationPermissions,
	MockTemplateResourceRun,
} from "#/testHelpers/templateResources";
import { FoundationTemplateDeleteDialog } from "./FoundationTemplateDeleteDialog";

const meta = {
	title: "pages/Foundations/TemplateDeletion",
	component: FoundationTemplateDeleteDialog,
	args: {
		templateId: MockTemplate.id,
		templateName: MockTemplate.name,
		onConfirm: fn(),
		onCancel: fn(),
	},
	parameters: {
		queries: [
			{
				key: foundationPermissionsKey(
					MockFoundation.organization_id,
					MockFoundation.id,
				),
				data: MockFoundationPermissions,
			},
		],
	},
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateFoundation").mockResolvedValue(
			MockFoundation,
		);
		return () => mock.mockRestore();
	},
} satisfies Meta<typeof FoundationTemplateDeleteDialog>;
export default meta;
type Story = StoryObj<typeof meta>;
export const CleanupRequired: Story = {};
export const WaitingForCleanup: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateFoundation").mockResolvedValue({
			...MockFoundation,
			status: "pending",
			runs: [
				{
					...MockTemplateResourceRun,
					action: "destroy",
					job: { ...MockTemplateResourceRun.job, status: "running" },
				},
			],
		});
		return () => mock.mockRestore();
	},
};
export const CleanupSucceeded: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateFoundation").mockResolvedValue({
			...MockFoundation,
			status: "destroyed",
		});
		return () => mock.mockRestore();
	},
};
export const UnknownState: Story = {
	beforeEach: () => {
		const mock = spyOn(API, "getTemplateFoundation").mockResolvedValue({
			...MockFoundation,
			status: "unknown",
		});
		return () => mock.mockRestore();
	},
};
