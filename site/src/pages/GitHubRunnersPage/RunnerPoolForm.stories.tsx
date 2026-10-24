import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { templatesByOrganizationKey } from "#/api/queries/templates";
import {
	MockDefaultOrganization,
	MockGitHubRunnerPool,
	MockTemplate,
	MockUserOwner,
} from "#/testHelpers/entities";
import { RunnerPoolForm } from "./RunnerPoolForm";

const meta = {
	title: "pages/GitHubRunnersPage/Form",
	component: RunnerPoolForm,
	decorators: [
		(Story) => (
			<div className="max-w-2xl p-6">
				<Story />
			</div>
		),
	],
	args: {
		organization: MockDefaultOrganization.id,
		defaultOwner: MockUserOwner,
		error: undefined,
		isPending: false,
		onSubmit: fn(),
		onCancel: fn(),
	},
} satisfies Meta<typeof RunnerPoolForm>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Create: Story = {
	parameters: {
		queries: [
			{
				key: templatesByOrganizationKey(MockDefaultOrganization.id),
				data: [MockTemplate],
			},
		],
	},
};
export const SubmissionError: Story = {
	parameters: {
		queries: [
			{
				key: templatesByOrganizationKey(MockDefaultOrganization.id),
				data: [MockTemplate],
			},
		],
	},
	args: {
		error: new Error(
			"Could not create runner pool. Check template permissions and retry.",
		),
	},
};
export const ValidationError: Story = {
	parameters: {
		queries: [
			{
				key: templatesByOrganizationKey(MockDefaultOrganization.id),
				data: [MockTemplate],
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Create pool" }),
		);
	},
};
export const TemplateLoading: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplatesByOrganization").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};
export const TemplateError: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplatesByOrganization").mockRejectedValue(
			new Error("Could not load templates. Check your connection."),
		);
	},
};
export const NoTemplates: Story = {
	parameters: {
		queries: [
			{ key: templatesByOrganizationKey(MockDefaultOrganization.id), data: [] },
		],
	},
};
export const Submitting: Story = {
	parameters: {
		queries: [
			{
				key: templatesByOrganizationKey(MockDefaultOrganization.id),
				data: [MockTemplate],
			},
		],
	},
	args: { isPending: true },
};
export const Edit: Story = { args: { pool: MockGitHubRunnerPool } };

export const AdvancedSettings: Story = {
	parameters: {
		queries: [
			{
				key: templatesByOrganizationKey(MockDefaultOrganization.id),
				data: [MockTemplate],
			},
		],
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(within(canvasElement).getByText("Advanced settings"));
	},
};
