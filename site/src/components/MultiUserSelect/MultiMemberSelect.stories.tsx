import type { Meta, StoryObj } from "@storybook/react-vite";
import { organizationMembersKey } from "#/api/queries/organizations";
import { MockOrganizationMember } from "#/testHelpers/entities";
import { MultiMemberSelect } from "./MultiUserSelect";

const meta: Meta<typeof MultiMemberSelect> = {
	title: "components/MultiMemberSelect",
	component: MultiMemberSelect,
};

export default meta;
type Story = StoryObj<typeof MultiMemberSelect>;

export const Loading: Story = {
	args: {
		organizationId: MockOrganizationMember.organization_id,
	},
	parameters: {
		queries: [
			{
				key: organizationMembersKey(MockOrganizationMember.organization_id, {
					limit: 25,
					q: "",
				}),
				data: {
					users: undefined,
					count: 0,
				},
			},
		],
	},
};
