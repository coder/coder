import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import {
	MockChatProject,
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { ChatProjectDialog } from "./ChatProjectDialog";

const meta = {
	title: "pages/AgentsPage/ChatsSidebar/ChatProjectDialog",
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

export const OpenOrganizationDropdown: Story = {
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("button", { name: /Organization/ }),
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
