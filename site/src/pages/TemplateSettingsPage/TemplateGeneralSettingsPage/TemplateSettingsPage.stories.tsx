import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import { templateByNameKey } from "#/api/queries/templates";
import { MockTemplate, mockApiError } from "#/testHelpers/entities";
import { withDashboardProvider, withToaster } from "#/testHelpers/storybook";
import { TemplateSettingsLayout } from "../TemplateSettingsLayout";
import TemplateSettingsPage from "./TemplateSettingsPage";

const meta = {
	title: "pages/TemplateSettingsPage/TemplateSettingsPage",
	component: TemplateSettingsLayout,
	decorators: [withToaster, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		reactRouter: reactRouterParameters({
			location: {
				path: "/templates/:template/settings",
				pathParams: { template: MockTemplate.name },
			},
			routing: [
				{
					path: "/templates/:template/settings",
					useStoryElement: true,
					children: [{ index: true, element: <TemplateSettingsPage /> }],
				},
				{ path: "/templates/:template", element: <div>Template</div> },
			],
		}),
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
	},
} satisfies Meta<typeof TemplateSettingsLayout>;

export default meta;
type Story = StoryObj<typeof meta>;

export const UpdateSucceeds: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const updateTemplateMetaSpy = spyOn(
			API,
			"updateTemplateMeta",
		).mockResolvedValue({ ...MockTemplate, name: "new-name" });
		await fillAndSubmitForm(canvas, user);
		await waitFor(() => expect(updateTemplateMetaSpy).toHaveBeenCalledTimes(1));
	},
};

export const DisplaysErrorWhenNameIsTaken: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const updateTemplateMetaSpy = spyOn(
			API,
			"updateTemplateMeta",
		).mockRejectedValue(
			mockApiError({
				message: `Template with name "test-template" already exists`,
				validations: [
					{
						field: "name",
						detail: "This value is already in use and should be unique.",
					},
				],
			}),
		);
		await fillAndSubmitForm(canvas, user);
		await waitFor(() => expect(updateTemplateMetaSpy).toHaveBeenCalledTimes(1));
		const form = await canvas.findByRole("form", {
			name: /template settings/i,
		});
		await within(form).findByText(
			"This value is already in use and should be unique.",
		);
	},
};

export const DeprecatesTemplateWithAccessControl: Story = {
	parameters: {
		features: ["access_control"],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const updateTemplateMetaSpy = spyOn(
			API,
			"updateTemplateMeta",
		).mockResolvedValue(MockTemplate);
		const deprecationMessage = "This template is deprecated";
		await deprecateTemplate(canvas, user, deprecationMessage);
		await waitFor(() => expect(updateTemplateMetaSpy).toHaveBeenCalledTimes(1));
		const [templateId, data] = updateTemplateMetaSpy.mock.calls[0];
		expect(templateId).toEqual(MockTemplate.id);
		expect(data).toEqual(
			expect.objectContaining({ deprecation_message: deprecationMessage }),
		);
	},
};

export const DoesNotDeprecateWithoutAccessControl: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		const updateTemplateMetaSpy = spyOn(
			API,
			"updateTemplateMeta",
		).mockResolvedValue(MockTemplate);
		await deprecateTemplate(
			canvas,
			user,
			"This template should not be able to deprecate",
		);
		await waitFor(() => expect(updateTemplateMetaSpy).toHaveBeenCalledTimes(1));
		const [templateId, data] = updateTemplateMetaSpy.mock.calls[0];
		expect(templateId).toEqual(MockTemplate.id);
		expect(data).toEqual(expect.objectContaining({ deprecation_message: "" }));
	},
};

async function fillAndSubmitForm(
	canvas: ReturnType<typeof within>,
	user: ReturnType<typeof userEvent.setup>,
) {
	const nameField = await canvas.findByLabelText("Name");
	await user.clear(nameField);
	await user.type(nameField, "Name");

	const displayNameField = await canvas.findByLabelText("Display name");
	await user.clear(displayNameField);
	await user.type(displayNameField, "A display name");

	const descriptionField = await canvas.findByLabelText("Description");
	await user.clear(descriptionField);
	await user.type(descriptionField, "A description");

	const iconField = await canvas.findByLabelText("Icon");
	await user.clear(iconField);
	await user.type(iconField, "vscode.png");

	const allowCancelJobsField = canvas.getByRole("checkbox", {
		name: /allow users to cancel in-progress workspace jobs/i,
	});
	await user.click(allowCancelJobsField);

	await user.click(await canvas.findByRole("button", { name: /save/i }));
}

async function deprecateTemplate(
	canvas: ReturnType<typeof within>,
	user: ReturnType<typeof userEvent.setup>,
	message: string,
) {
	const deprecationField = await canvas.findByLabelText("Deprecation Message");
	await user.type(deprecationField, message);
	await user.click(await canvas.findByRole("button", { name: /save/i }));
}
