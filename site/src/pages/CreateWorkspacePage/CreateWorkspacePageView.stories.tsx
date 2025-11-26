import { chromatic } from "testHelpers/chromatic";
import { MockTemplate, MockUserOwner } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { DetailedError } from "api/errors";
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
		versionId: "template-version-123",
		template: {
			...MockTemplate,
			organization_name: "default",
			name: "docker-template",
		},
	},
	parameters: {
		docs: {
			description: {
				story:
					"This story shows the View Source button that appears for template administrators in the experimental workspace creation page. The button allows quick navigation to the template editor.",
			},
		},
	},
};
