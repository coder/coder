import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { agentTemplates } from "#/api/queries/templates";
import type { Template } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockDefaultOrganization,
	MockOrganization2,
	MockTemplate,
	mockApiError,
} from "#/testHelpers/entities";
import { ChatProjectDialog } from "./ChatProjectDialog";

const dockerTemplate: Template = {
	...MockTemplate,
	id: "template-docker",
	name: "docker",
	display_name: "Docker",
	active_version_id: "version-docker",
};
const kubernetesTemplate: Template = {
	...MockTemplate,
	id: "template-kubernetes",
	name: "kubernetes",
	display_name: "Kubernetes",
	active_version_id: "version-kubernetes",
};
const meta = {
	title: "pages/AgentsPage/ChatProjectDialog",
	component: ChatProjectDialog,
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
		// Every story with an organization loads its templates.
		queries: [MockDefaultOrganization, MockOrganization2].map(
			(organization) => ({
				key: agentTemplates(organization.id).queryKey,
				data: [dockerTemplate, kubernetesTemplate],
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
			message: `Template ${kubernetesTemplate.id} is not available to agents in this organization.`,
			validations: [
				{
					field: "templates.template_ids",
					detail: `Template ${kubernetesTemplate.id} is not available to agents in this organization.`,
				},
			],
		}),
	},
};

export const NoEligibleTemplates: Story = {
	parameters: {
		queries: [
			{
				key: agentTemplates(MockDefaultOrganization.id).queryKey,
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
