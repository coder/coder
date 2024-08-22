import type { Meta, StoryObj } from "@storybook/react";
import { MockOrganizationMember } from "testHelpers/entities";
import { MemberAutocomplete } from "./MemberAutocomplete";

const meta: Meta<typeof MemberAutocomplete> = {
	title: "components/MemberAutocomplete",
	component: MemberAutocomplete,
};

export default meta;
type Story = StoryObj<typeof MemberAutocomplete>;

export const SelectedMember: Story = {
	args: {
		value: MockOrganizationMember,
		organizationId: MockOrganizationMember.organization_id,
	},
};
