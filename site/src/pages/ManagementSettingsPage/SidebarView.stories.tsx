import type { Meta, StoryObj } from "@storybook/react";
import {
  MockOrganization,
  MockOrganization2,
  MockPermissions,
} from "testHelpers/entities";
import { SidebarView } from "./SidebarView";

const meta: Meta<typeof SidebarView> = {
  title: "components/MultiOrgSidebarView",
  component: SidebarView,
  args: {
    activeOrganization: undefined,
    activeOrgPermissions: undefined,
    organizations: [MockOrganization, MockOrganization2],
    permissions: MockPermissions,
  },
};

export default meta;
type Story = StoryObj<typeof SidebarView>;

export const Default: Story = {};

export const NoCreateOrg: Story = {
  args: {
    permissions: {
      ...MockPermissions,
      createOrganization: false,
    },
  },
};

export const NoViewUsers: Story = {
  args: {
    permissions: {
      ...MockPermissions,
      viewAllUsers: false,
    },
  },
};

export const NoAuditLog: Story = {
  args: {
    permissions: {
      ...MockPermissions,
      viewAnyAuditLog: false,
    },
  },
};

export const NoDeploymentValues: Story = {
  args: {
    permissions: {
      ...MockPermissions,
      viewDeploymentValues: false,
      editDeploymentValues: false,
    },
  },
};

export const NoPermissions: Story = {
  args: {
    permissions: {},
  },
};

export const SelectedOrgLoading: Story = {
  args: {
    activeOrganization: MockOrganization,
  },
};

export const SelectedOrgAdmin: Story = {
  args: {
    activeOrganization: MockOrganization,
    activeOrgPermissions: {
      editOrganization: true,
      viewMembers: true,
      viewGroups: true,
      auditOrganization: true,
    },
  },
};

export const SelectedOrgAuditor: Story = {
  args: {
    activeOrganization: MockOrganization,
    activeOrgPermissions: {
      editOrganization: false,
      viewMembers: false,
      viewGroups: false,
      auditOrganization: true,
    },
  },
};

export const SelectedOrgNoPerms: Story = {
  args: {
    activeOrganization: MockOrganization,
    activeOrgPermissions: {
      editOrganization: false,
      viewMembers: false,
      viewGroups: false,
      auditOrganization: false,
    },
  },
};
