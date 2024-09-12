import type { Meta, StoryObj } from "@storybook/react";
import { MockOIDCConfig } from "testHelpers/entities";
import { IdpSyncPageView } from "./IdpSyncPageView";

const meta: Meta<typeof IdpSyncPageView> = {
	title: "pages/OrganizationIdpSyncPage",
	component: IdpSyncPageView,
};

export default meta;
type Story = StoryObj<typeof IdpSyncPageView>;

export const Empty: Story = {
	args: { oidcConfig: undefined },
};

export const Default: Story = {
	args: { oidcConfig: MockOIDCConfig },
};
