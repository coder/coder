import { chromatic } from "testHelpers/chromatic";
import { MockTemplate, MockUserOwner } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { DetailedError } from "api/errors";
import { expect, screen, within } from "storybook/test";
import { CreateWorkspacePageView } from "./CreateWorkspacePageView";

const meta: Meta<typeof CreateWorkspacePageView> = {
	title: "Pages/CreateWorkspacePageView",
	parameters: { chromatic },
	component: CreateWorkspacePageView,
	args: {
		autofillParameters: [],
		diagnostics: [],
		defaultName: "",
		defaultOwner: MockUserOwner,
		externalAuth: [],
		externalAuthPollingState: "idle",
		hasAllRequiredExternalAuth: true,
		mode: "form",
		parameters: [],
		permissions: {
			createWorkspaceForAny: true,
			canUpdateTemplate: false,
		},
		presets: [],
		sendMessage: () => {},
		template: MockTemplate,
	},
};

export default meta;
type Story = StoryObj<typeof CreateWorkspacePageView>;

export const WebsocketError: Story = {
	args: {
		error: new DetailedError(
			"Websocket connection for dynamic parameters unexpectedly closed.",
			"Refresh the page to reset the form.",
		),
	},
};

export const WithViewSourceButton: Story = {
	args: {
		canUpdateTemplate: true,
		versionName: "foobar-template-version",
		template: {
			...MockTemplate,
			organization_name: "default",
			name: "docker-template",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const viewSourceLink = canvas.getByRole("link", { name: /view source/i });
		expect(viewSourceLink).toBeInTheDocument();
		expect(viewSourceLink).toHaveAttribute(
			"href",
			"/templates/default/docker-template/versions/foobar-template-version/edit",
		);
	},
};

export const ViewSourceButtonHiddenWithoutPermission: Story = {
	args: {
		canUpdateTemplate: false,
		versionName: "foobar-template-version",
	},
	play: async () => {
		expect(
			screen.queryByRole("link", { name: /view source/i }),
		).not.toBeInTheDocument();
	},
};
