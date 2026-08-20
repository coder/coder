import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import { templateByNameKey, templateVersion } from "#/api/queries/templates";
import {
	MockTemplate,
	MockTemplateVersion,
	MockTemplateVersion2,
	mockApiError,
} from "#/testHelpers/entities";
import { withDashboardProvider, withToaster } from "#/testHelpers/storybook";
import { TemplateSettingsLayout } from "../TemplateSettingsLayout";
import TemplateParametersPage from "./TemplateParametersPage";

const meta = {
	title: "pages/TemplateSettingsPage/TemplateParametersPage",
	component: TemplateSettingsLayout,
	decorators: [withToaster, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		pixel: { exclude: true },
		reactRouter: reactRouterParameters({
			location: {
				path: "/templates/:template/settings/parameters",
				pathParams: { template: MockTemplate.name },
			},
			routing: [
				{
					path: "/templates/:template/settings",
					useStoryElement: true,
					children: [
						{ path: "parameters", element: <TemplateParametersPage /> },
					],
				},
			],
		}),
		queries: [
			{
				key: templateByNameKey("default", MockTemplate.name),
				data: MockTemplate,
			},
			{
				key: templateVersion(MockTemplate.active_version_id).queryKey,
				data: MockTemplateVersion,
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
	},
} satisfies Meta<typeof TemplateSettingsLayout>;

export default meta;
type Story = StoryObj<typeof meta>;

const confirmRefresh = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	const user = userEvent.setup();

	await user.click(
		await canvas.findByRole("button", { name: /refresh template data/i }),
	);
	const dialog = within(await within(document.body).findByRole("dialog"));
	await user.click(dialog.getByRole("button", { name: "Refresh" }));
};

export const DisablesDynamicParameters: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const updateTemplateMetaSpy = spyOn(
			API,
			"updateTemplateMeta",
		).mockResolvedValue(MockTemplate);

		const checkbox = await canvas.findByRole("checkbox", {
			name: /enable dynamic parameters for workspace creation/i,
		});
		await expect(checkbox).toBeChecked();
		await user.click(checkbox);

		await waitFor(() =>
			expect(updateTemplateMetaSpy).toHaveBeenCalledWith(MockTemplate.id, {
				use_classic_parameter_flow: true,
			}),
		);
		await within(document.body).findByText("Dynamic parameters disabled.");
	},
};

export const RefreshCreatesAndPublishesAVersion: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const createTemplateVersionSpy = spyOn(
			API,
			"createTemplateVersion",
		).mockResolvedValue(MockTemplateVersion2);
		spyOn(API, "getTemplateVersion").mockResolvedValue(MockTemplateVersion2);
		spyOn(API, "getTemplateByName").mockResolvedValue({
			...MockTemplate,
			active_version_id: MockTemplateVersion2.id,
		});
		const updateActiveTemplateVersionSpy = spyOn(
			API,
			"updateActiveTemplateVersion",
		).mockResolvedValue({ message: "ok" });

		await canvas.findByText(MockTemplateVersion.name);
		await confirmRefresh(canvasElement);

		await waitFor(() =>
			expect(createTemplateVersionSpy).toHaveBeenCalledWith(
				"default",
				expect.objectContaining({
					template_id: MockTemplate.id,
					file_id: MockTemplateVersion.job.file_id,
					storage_method: "file",
					provisioner: "terraform",
				}),
			),
		);

		await waitFor(
			() =>
				expect(updateActiveTemplateVersionSpy).toHaveBeenCalledWith(
					MockTemplate.id,
					{ id: MockTemplateVersion2.id },
				),
			{ timeout: 5000 },
		);

		await within(document.body).findByText(
			`Template "${MockTemplate.name}" data refreshed successfully.`,
		);
		await canvas.findByText(MockTemplateVersion2.name);
	},
};

export const PublishFails: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "createTemplateVersion").mockResolvedValue(MockTemplateVersion2);
		spyOn(API, "getTemplateVersion").mockResolvedValue(MockTemplateVersion2);
		spyOn(API, "updateActiveTemplateVersion").mockRejectedValue(
			mockApiError({ message: "Failed to update active template version." }),
		);

		await confirmRefresh(canvasElement);

		await within(canvasElement).findByText(
			"Failed to update active template version.",
			undefined,
			{ timeout: 5000 },
		);
	},
};
