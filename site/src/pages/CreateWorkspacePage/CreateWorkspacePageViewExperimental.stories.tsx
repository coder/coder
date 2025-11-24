import { chromatic } from "testHelpers/chromatic";
import { MockTemplate, MockUserOwner } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { DetailedError } from "api/errors";
import { CreateWorkspacePageViewExperimental } from "./CreateWorkspacePageViewExperimental";

const meta: Meta<typeof CreateWorkspacePageViewExperimental> = {
	title: "Pages/CreateWorkspacePageViewExperimental",
	parameters: { chromatic },
	component: CreateWorkspacePageViewExperimental,
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
type Story = StoryObj<typeof CreateWorkspacePageViewExperimental>;

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
