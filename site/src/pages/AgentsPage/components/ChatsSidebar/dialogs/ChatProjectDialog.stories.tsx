import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, screen, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import {
	agentTemplatesKey,
	dynamicParametersKey,
	templateVersionPresetsKey,
} from "#/api/queries/templates";
import type {
	DynamicParametersResponse,
	PreviewParameter,
	Template,
} from "#/api/typesGenerated";
import {
	MockChatProject,
	MockDefaultOrganization,
	MockDropdownParameter,
	MockOrganization2,
	MockSliderParameter,
	MockSwitchParameter,
	MockTemplate,
	MockUserOwner,
	MockValidationParameter,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { ChatProjectDialog } from "./ChatProjectDialog";

const mockDockerTemplate: Template = {
	...MockTemplate,
	id: "template-docker",
	name: "docker",
	display_name: "Docker",
	active_version_id: "version-docker",
};
const mockKubernetesTemplate: Template = {
	...MockTemplate,
	id: "template-kubernetes",
	name: "kubernetes",
	display_name: "Kubernetes",
	active_version_id: "version-kubernetes",
};
// An evaluation fills each value with the parameter's default.
const mockInstanceTypeParameter: PreviewParameter = {
	...MockDropdownParameter,
	value: MockDropdownParameter.default_value,
};
const mockParameters: DynamicParametersResponse = {
	id: 0,
	diagnostics: [],
	parameters: [
		mockInstanceTypeParameter,
		MockSliderParameter,
		MockSwitchParameter,
	],
};
const mockOutOfRangeParameter: PreviewParameter = {
	...MockValidationParameter,
	value: { value: "500", valid: true },
	diagnostics: [
		{
			severity: "error",
			summary: "Number must be between 0 and 100",
			detail: "",
			extra: { code: "" },
		},
	],
};
const meta = {
	title: "pages/AgentsPage/ChatProjectDialog",
	component: ChatProjectDialog,
	decorators: [withAuthProvider],
	args: {
		open: true,
		organizations: [MockDefaultOrganization, MockOrganization2],
		initialOrganizationId: MockDefaultOrganization.id,
		onOpenChange: fn(),
		isSubmitting: false,
		error: undefined,
		onSubmit: fn(),
	},
	parameters: {
		// The default parameter values are evaluated as this user.
		user: MockUserOwner,
		queries: [MockDefaultOrganization, MockOrganization2].map(
			(organization) => ({
				key: agentTemplatesKey(organization.id),
				data: [mockDockerTemplate, mockKubernetesTemplate],
			}),
		),
	},
} satisfies Meta<typeof ChatProjectDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const OpenOrganizationDropdown: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("combobox", { name: /Organization/ }),
		);
	},
};

export const SingleOrganization: Story = {
	args: {
		organizations: [MockOrganization2],
		initialOrganizationId: MockOrganization2.id,
	},
};

export const UnavailableOrganizations: Story = {
	args: {
		organizations: [],
		initialOrganizationId: undefined,
	},
};

export const UnavailableSelection: Story = {
	args: {
		organizations: [MockOrganization2],
	},
};

export const NameRequired: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(body.getByLabelText(/Project name/));
		await userEvent.tab();
	},
};

export const InvalidName: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.type(
			await body.findByRole("textbox", { name: /Project name/ }),
			"x".repeat(65),
		);
	},
};

export const NameFieldError: Story = {
	args: {
		project: MockChatProject,
		error: mockApiError({
			message: "Name must be at most 64 characters.",
			validations: [
				{ field: "name", detail: "Name must be at most 64 characters." },
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(body.getByLabelText(/Name/));
		await userEvent.tab();
	},
};

export const Submitting: Story = {
	render: function Render(args) {
		const [isSubmitting, setIsSubmitting] = useState(false);
		return (
			<ChatProjectDialog
				{...args}
				isSubmitting={isSubmitting}
				onSubmit={() => setIsSubmitting(true)}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.type(
			await body.findByRole("textbox", { name: /Project name/ }),
			MockChatProject.name,
		);
		await userEvent.click(body.getByRole("button", { name: "Create project" }));
	},
};

export const MutationError: Story = {
	args: {
		error: new Error(
			"You do not have permission to create a project in this organization.",
		),
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.type(
			await body.findByRole("textbox", { name: /Project name/ }),
			MockChatProject.name,
		);
		await userEvent.type(
			body.getByRole("textbox", { name: "Description" }),
			MockChatProject.description,
		);
	},
};

export const Edit: Story = {
	args: {
		project: MockChatProject,
	},
};

export const Narrow: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
};

export const NarrowOpenOrganizationDropdown: Story = {
	...OpenOrganizationDropdown,
	globals: Narrow.globals,
};

/** The server rejected the templates, for example one deprecated meanwhile. */
export const CreateTemplatesSaveError: Story = {
	args: {
		error: mockApiError({
			message: `Template ${mockKubernetesTemplate.id} is not available to agents in this organization.`,
			validations: [
				{
					field: "template_ids",
					detail: `Template ${mockKubernetesTemplate.id} is not available to agents in this organization.`,
				},
			],
		}),
	},
};

export const LimitWithoutTemplates: Story = {
	// Turns the limit on so the screenshot shows the warning for no templates.
	play: async () => {
		await userEvent.click(
			await screen.findByRole("switch", {
				name: "Limit chats to these templates",
			}),
		);
	},
};

export const NoEligibleTemplates: Story = {
	parameters: {
		queries: [
			{
				key: agentTemplatesKey(MockDefaultOrganization.id),
				data: [],
			},
		],
	},
	// Opens the templates list so the screenshot shows its empty message.
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("combobox", { name: "Templates" }),
		);
	},
};

export const TemplatesLoadError: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API, "getTemplates").mockRejectedValue(
			mockApiError({ message: "Failed to load templates." }),
		);
	},
};

const pickDefaultTemplate: Story["play"] = async () => {
	await userEvent.click(
		await screen.findByRole("button", { name: "Select all" }),
	);
	await userEvent.click(
		screen.getByRole("combobox", { name: "Default template" }),
	);
	await userEvent.click(await screen.findByRole("option", { name: "Docker" }));
};

const pickDefaultParameters: Story["play"] = async (context) => {
	await pickDefaultTemplate(context);
	await userEvent.click(
		await screen.findByRole("radio", { name: "Parameter values" }),
	);
};

export const DefaultTemplateWithoutPresets: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{
				key: templateVersionPresetsKey(mockDockerTemplate.active_version_id),
				data: [],
			},
		],
	},
	play: pickDefaultTemplate,
};

export const DefaultTemplatePresetsLoadError: Story = {
	beforeEach: () => {
		spyOn(API, "getTemplateVersionPresets").mockRejectedValue(
			mockApiError({ message: "Failed to load presets." }),
		);
	},
	play: pickDefaultTemplate,
};

export const DefaultTemplateParameters: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{
				key: templateVersionPresetsKey(mockDockerTemplate.active_version_id),
				data: [],
			},
			{
				key: dynamicParametersKey(
					mockDockerTemplate.active_version_id,
					MockUserOwner.id,
					{},
				),
				data: mockParameters,
			},
			{
				key: dynamicParametersKey(
					mockDockerTemplate.active_version_id,
					MockUserOwner.id,
					{ [MockSwitchParameter.name]: "false" },
				),
				data: mockParameters,
			},
		],
	},
	// Changes one value so the screenshot shows a project default, which can
	// be reset, next to parameters that keep the template default.
	play: async (context) => {
		await pickDefaultParameters(context);
		await userEvent.click(
			await screen.findByRole("switch", {
				name: MockSwitchParameter.display_name,
			}),
		);
	},
};

/** The evaluation reports a problem with the template and with a value. */
export const DefaultTemplateParameterDiagnostics: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{
				key: templateVersionPresetsKey(mockDockerTemplate.active_version_id),
				data: [],
			},
			{
				key: dynamicParametersKey(
					mockDockerTemplate.active_version_id,
					MockUserOwner.id,
					{},
				),
				data: {
					id: 0,
					diagnostics: [
						{
							severity: "warning",
							summary:
								"This template version is missing required metadata to support dynamic parameters.",
							detail:
								"To restore full functionality, please re-import the terraform as a new template version.",
							extra: { code: "" },
						},
					],
					parameters: [mockOutOfRangeParameter],
				} satisfies DynamicParametersResponse,
			},
		],
	},
	play: pickDefaultParameters,
};

export const DefaultTemplateWithoutParameters: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{
				key: templateVersionPresetsKey(mockDockerTemplate.active_version_id),
				data: [],
			},
			{
				key: dynamicParametersKey(
					mockDockerTemplate.active_version_id,
					MockUserOwner.id,
					{},
				),
				data: {
					id: 0,
					diagnostics: [],
					parameters: [],
				} satisfies DynamicParametersResponse,
			},
		],
	},
	play: pickDefaultParameters,
};

export const DefaultTemplateParametersLoadError: Story = {
	parameters: {
		queries: [
			...meta.parameters.queries,
			{
				key: templateVersionPresetsKey(mockDockerTemplate.active_version_id),
				data: [],
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "getTemplateVersionDynamicParameters").mockRejectedValue(
			mockApiError({ message: "Failed to load parameters." }),
		);
	},
	play: pickDefaultParameters,
};
