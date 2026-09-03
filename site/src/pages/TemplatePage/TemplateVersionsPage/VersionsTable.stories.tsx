import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { within } from "storybook/test";
import {
	MockCanceledProvisionerJob,
	MockCancelingProvisionerJob,
	MockFailedProvisionerJob,
	MockNoPermissions,
	MockPendingProvisionerJob,
	MockPermissions,
	MockRunningProvisionerJob,
	MockTemplateVersion,
	MockUserOwner,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { VersionsTable } from "./VersionsTable";

const meta: Meta<typeof VersionsTable> = {
	title: "pages/TemplatePage/VersionsTable",
	component: VersionsTable,
	parameters: {
		user: MockUserOwner,
		permissions: MockPermissions,
	},
	args: {
		onPromoteClick: () => {},
		onArchiveClick: () => {},
	},
	decorators: [withAuthProvider],
};

export default meta;
type Story = StoryObj<typeof VersionsTable>;

export const Example: Story = {
	args: {
		activeVersionId: MockTemplateVersion.id,
		versions: [
			{
				...MockTemplateVersion,
				id: "2",
				name: "test-template-version-2",
				created_at: "2022-05-18T18:39:01.382927298Z",
			},
			MockTemplateVersion,
		],
	},
	play: async ({ canvasElement }) => {
		// With permissions.updateTemplates, VersionRow is a clickable button
		const canvas = within(canvasElement);
		await canvas.findByRole("button", { name: MockTemplateVersion.name });
	},
};

export const NoUpdatePermission: Story = {
	args: { ...Example.args },
	parameters: {
		permissions: MockNoPermissions,
	},
	play: async ({ canvasElement }) => {
		// Without permissions.updateTemplates, VersionRow is a non-clickable row
		const canvas = within(canvasElement);
		await canvas.findByRole("row", { name: MockTemplateVersion.name });
	},
};

export const NoEditPermission: Story = {
	args: {
		...Example.args,
		onPromoteClick: undefined,
		onArchiveClick: undefined,
	},
};

export const BuildStatuses: Story = {
	args: {
		activeVersionId: MockTemplateVersion.id,
		onPromoteClick: action("onPromoteClick"),
		versions: [
			{
				...MockTemplateVersion,
				id: "6",
				name: "test-version-6",
				created_at: "2022-05-18T18:39:01.382927298Z",
				job: MockCancelingProvisionerJob,
			},
			{
				...MockTemplateVersion,
				id: "5",
				name: "test-version-5",
				created_at: "2022-05-18T18:39:01.382927298Z",
				job: MockCanceledProvisionerJob,
			},
			{
				...MockTemplateVersion,
				id: "4",
				name: "test-version-4",
				created_at: "2022-05-18T18:39:01.382927298Z",
				job: MockRunningProvisionerJob,
			},
			{
				...MockTemplateVersion,
				id: "3",
				name: "test-version-3",
				created_at: "2022-05-18T18:39:01.382927298Z",
				job: MockPendingProvisionerJob,
			},
			{
				...MockTemplateVersion,
				id: "2",
				name: "test-version-2",
				created_at: "2022-05-18T18:39:01.382927298Z",
				job: MockFailedProvisionerJob,
			},
			MockTemplateVersion,
		],
	},
};

export const BuildStatusesNoEditPermission: Story = {
	args: {
		...BuildStatuses.args,
		onPromoteClick: undefined,
		onArchiveClick: undefined,
	},
};

export const Empty: Story = {
	args: {
		versions: [],
	},
};
