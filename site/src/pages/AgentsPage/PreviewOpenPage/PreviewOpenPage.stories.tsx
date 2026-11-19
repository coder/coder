import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { workspaceByOwnerAndNameKey } from "#/api/queries/workspaces";
import { MockPreviewSignInResponse } from "#/testHelpers/chatEntities";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	mockApiError,
} from "#/testHelpers/entities";
import { withProxyProvider } from "#/testHelpers/storybook";
import PreviewOpenPage from "./PreviewOpenPage";

const meta = {
	title: "pages/AgentsPage/PreviewOpenPage",
	component: PreviewOpenPage,
	decorators: [withProxyProvider()],
	parameters: {
		layout: "fullscreen",
		reactRouter: reactRouterParameters({
			location: {
				path: "/previews/open",
				searchParams: {
					owner: MockWorkspace.owner_name,
					workspace: MockWorkspace.name,
					agent: MockWorkspaceAgent.name,
					slug: "preview--3000",
				},
			},
			routing: { path: "/previews/open" },
		}),
		queries: [
			{
				key: workspaceByOwnerAndNameKey(
					MockWorkspace.owner_name,
					MockWorkspace.name,
				),
				data: MockWorkspace,
			},
		],
	},
	beforeEach: () => {
		// A blank page keeps the screenshot independent of the network.
		spyOn(API.experimental, "signInToPreview").mockResolvedValue({
			...MockPreviewSignInResponse,
			url: "about:blank",
		});
	},
} satisfies Meta<typeof PreviewOpenPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Ready: Story = {};

export const LoadingWorkspace: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API, "getWorkspaceByOwnerAndName").mockReturnValue(
			new Promise(() => {}),
		);
	},
};

export const WorkspaceNotFound: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API, "getWorkspaceByOwnerAndName").mockRejectedValue(
			mockApiError({
				message:
					"Resource not found or you do not have access to this resource",
			}),
		);
	},
};

export const IncompleteLink: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/previews/open",
				searchParams: { owner: MockWorkspace.owner_name },
			},
			routing: { path: "/previews/open" },
		}),
	},
};
