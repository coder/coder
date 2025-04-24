import type { Meta, StoryObj } from "@storybook/react";
import { DetailedError } from "api/errors";
import { chromatic } from "testHelpers/chromatic";
import { MockTemplate, MockUser } from "testHelpers/entities";
import { CreateWorkspacePageViewExperimental } from "./CreateWorkspacePageViewExperimental";

const meta: Meta<typeof CreateWorkspacePageViewExperimental> = {
	title: "Pages/CreateWorkspacePageViewExperimental",
	parameters: { chromatic },
	component: CreateWorkspacePageViewExperimental,
	args: {
		autofillParameters: [],
		diagnostics: [],
		defaultName: "",
		defaultOwner: MockUser,
		externalAuth: [],
		externalAuthPollingState: "idle",
		hasAllRequiredExternalAuth: true,
		mode: "form",
		parameters: [],
		permissions: {
			createWorkspaceForAny: true,
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
