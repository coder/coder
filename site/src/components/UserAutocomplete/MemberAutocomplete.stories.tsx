import type { Meta, StoryObj } from "@storybook/react";
import { MockOrganizationMember } from "testHelpers/entities";
import { MemberAutocomplete } from "./UserAutocomplete";

const meta: Meta<typeof MemberAutocomplete> = {
	title: "components/MemberAutocomplete",
	component: MemberAutocomplete,
};

export default meta;
type Story = StoryObj<typeof MemberAutocomplete>;

export const WithLabel: Story = {
	args: {
		value: MockOrganizationMember,
		organizationId: MockOrganizationMember.organization_id,
		label: "Member",
	},
};

export const NoLabel: Story = {
	args: {
		value: MockOrganizationMember,
		organizationId: MockOrganizationMember.organization_id,
	},
};
