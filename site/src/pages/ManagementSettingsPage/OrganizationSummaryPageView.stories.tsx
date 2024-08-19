import type { Meta, StoryObj } from "@storybook/react";
import {
	MockDefaultOrganization,
	MockOrganization,
} from "testHelpers/entities";
import { OrganizationSummaryPageView } from "./OrganizationSummaryPageView";

const meta: Meta<typeof OrganizationSummaryPageView> = {
	title: "pages/OrganizationSummaryPageView",
	component: OrganizationSummaryPageView,
	args: {
		organization: MockOrganization,
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationSummaryPageView>;

export const DefaultOrg: Story = {
	args: {
		organization: MockDefaultOrganization,
	},
};
