import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
import { organizationSkillACLKey } from "#/api/queries/skills";
import {
	MockDefaultOrganization,
	MockGroup,
	MockOrganizationSkillACL,
	mockApiError,
} from "#/testHelpers/entities";
import { MockSkills } from "#/testHelpers/skills";
import { OrganizationSkillSharingDialog } from "./OrganizationSkillSharingDialog";

const mockSkillName = MockSkills[0].name;

const meta: Meta<typeof OrganizationSkillSharingDialog> = {
	title: "pages/AISettingsPage/SkillsPage/OrganizationSkillSharingDialog",
	component: OrganizationSkillSharingDialog,
	args: {
		organizationId: MockDefaultOrganization.id,
		skillName: mockSkillName,
		onClose: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationSkillSharingDialog>;

export const Populated: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getOrganizationSkillACL").mockResolvedValue(
			MockOrganizationSkillACL,
		);
	},
};

export const Empty: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getOrganizationSkillACL").mockResolvedValue({
			users: [],
			groups: [],
		});
	},
};

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getOrganizationSkillACL").mockReturnValue(
			new Promise(() => undefined),
		);
	},
};

export const LoadError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getOrganizationSkillACL").mockRejectedValue(
			mockApiError({ message: "Failed to load skill permissions." }),
		);
	},
};

export const RefetchError: Story = {
	parameters: {
		queries: [
			{
				key: organizationSkillACLKey(MockDefaultOrganization.id, mockSkillName),
				data: MockOrganizationSkillACL,
			},
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getOrganizationSkillACL").mockRejectedValue(
			mockApiError({ message: "Failed to refresh skill permissions." }),
		);
	},
};

export const SaveError: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getOrganizationSkillACL").mockResolvedValue(
			MockOrganizationSkillACL,
		);
		spyOn(API.experimental, "updateOrganizationSkillACL").mockRejectedValue(
			mockApiError({ message: "Failed to save skill permissions." }),
		);
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", {
				name: `Remove ${MockGroup.display_name}`,
			}),
		);
		await userEvent.click(
			screen.getByRole("button", { name: "Save permissions" }),
		);
		await screen.findByText("Failed to save skill permissions.");
	},
};
