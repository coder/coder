import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import {
	MockChatProject,
	MockDefaultOrganization,
	MockOrganization2,
	mockApiError,
} from "#/testHelpers/entities";
import { ChatProjectDialog } from "./ChatProjectDialog";

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
} satisfies Meta<typeof ChatProjectDialog>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

/** A new agent can start with the built-in MCP servers its organization has. */
export const WithQuickAddMCPServers: Story = {
	args: {
		quickAddMCPServers: {
			[MockDefaultOrganization.id]: [
				{ id: "slack", label: "Slack", iconURL: "/icon/slack.svg" },
				{ id: "github", label: "GitHub", iconURL: "/icon/github.svg" },
			],
		},
	},
};

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
		await userEvent.click(body.getByLabelText(/Agent name/));
		await userEvent.tab();
	},
};

export const InvalidName: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.type(
			await body.findByRole("textbox", { name: /Agent name/ }),
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
			await body.findByRole("textbox", { name: /Agent name/ }),
			MockChatProject.name,
		);
		await userEvent.click(body.getByRole("button", { name: "Create agent" }));
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
			await body.findByRole("textbox", { name: /Agent name/ }),
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
