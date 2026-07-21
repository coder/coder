import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type { ChatModelConfigACL } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockGroup,
	MockUserMember,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { mockGPT5 } from "../testFixtures";
import ModelSharingPage from "./ModelSharingPage";

const model = {
	...mockGPT5,
	organization_id: MockDefaultOrganization.id,
};
const acl: ChatModelConfigACL = {
	users: [
		{
			id: MockUserMember.id,
			username: MockUserMember.username,
			name: MockUserMember.name,
			avatar_url: MockUserMember.avatar_url,
			role: "read",
		},
	],
	groups: [{ ...MockGroup, role: "read" }],
};

const mockRequests = ({ canEdit = true, canShare = true } = {}) => {
	spyOn(API.experimental, "getChatModelConfig").mockResolvedValue(model);
	spyOn(API.experimental, "getChatModelConfigACL").mockResolvedValue(acl);
	spyOn(API.experimental, "updateChatModelConfigACL").mockResolvedValue();
	spyOn(API, "checkAuthorization").mockResolvedValue({ canEdit, canShare });
};

const meta: Meta<typeof ModelSharingPage> = {
	title: "pages/AISettingsPage/ModelsPage/ModelSharingPage",
	component: ModelSharingPage,
	decorators: [withDashboardProvider],
	parameters: {
		features: ["template_rbac"],
		reactRouter: reactRouterParameters({
			location: {
				path: `/ai/settings/models/${model.id}/sharing`,
				searchParams: { organization: "wrong-organization" },
			},
			routing: [
				{ path: "/ai/settings/models/:modelId/sharing", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof ModelSharingPage>;

export const Default: Story = {
	beforeEach: () => mockRequests(),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.findByText(`Share ${model.display_name}`),
		).resolves.toBeVisible();
		await expect(
			canvas.findByText(MockUserMember.username),
		).resolves.toBeVisible();
		await waitFor(() => {
			expect(
				canvas.getByRole("link", { name: "Back to model" }),
			).toHaveAttribute(
				"href",
				`/ai/settings/models/${model.id}?organization=${MockDefaultOrganization.name}`,
			);
		});
		expect(API.experimental.getChatModelConfig).toHaveBeenCalledWith(model.id);
		expect(API.experimental.getChatModelConfigACL).toHaveBeenCalledWith(
			model.id,
		);
	},
};

export const NoSharePermission: Story = {
	beforeEach: () => mockRequests({ canEdit: false, canShare: false }),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.findByText(/you do not have permission to change them/i),
		).resolves.toBeVisible();
		await expect(
			canvas.findByRole("link", { name: "Back to model" }),
		).resolves.toHaveAttribute(
			"href",
			`/ai/settings/models?organization=${MockDefaultOrganization.name}`,
		);
	},
};

export const EntitlementPaywall: Story = {
	parameters: { features: [] },
	beforeEach: () => {
		spyOn(API.experimental, "getChatModelConfig").mockResolvedValue(model);
		spyOn(API.experimental, "getChatModelConfigACL").mockResolvedValue(acl);
		spyOn(API, "checkAuthorization").mockResolvedValue({
			canEdit: true,
			canShare: true,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.findByText("Model sharing")).resolves.toBeVisible();
		expect(API.experimental.getChatModelConfigACL).not.toHaveBeenCalled();
		expect(API.checkAuthorization).not.toHaveBeenCalled();
	},
};
