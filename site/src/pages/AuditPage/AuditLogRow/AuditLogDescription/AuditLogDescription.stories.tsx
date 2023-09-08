import type { Meta, StoryObj } from "@storybook/react";
import { AuditLogDescription } from "./AuditLogDescription";
import {
  MockAuditLog,
  MockAuditLogSuccessfulLogin,
  MockAuditLogUnsuccessfulLoginKnownUser,
  MockAuditLogWithWorkspaceBuild,
  MockWorkspaceCreateAuditLogForDifferentOwner,
} from "testHelpers/entities";

const meta: Meta<typeof AuditLogDescription> = {
  title: "components/AuditLogDescription",
  component: AuditLogDescription,
};

export default meta;
type Story = StoryObj<typeof AuditLogDescription>;

export const WorkspaceCreate: Story = {
  args: {
    auditLog: MockAuditLog,
  },
};

export const WorkspaceBuildStop: Story = {
  args: {
    auditLog: MockAuditLogWithWorkspaceBuild,
  },
};

export const WorkspaceBuildDuplicatedWord: Story = {
  args: {
    auditLog: {
      ...MockAuditLogWithWorkspaceBuild,
      additional_fields: {
        workspace_name: "workspace",
      },
    },
  },
};

export const CreateWorkspaceWithDiffOwner: Story = {
  args: {
    auditLog: MockWorkspaceCreateAuditLogForDifferentOwner,
  },
};

export const SuccessLogin: Story = {
  args: {
    auditLog: MockAuditLogSuccessfulLogin,
  },
};

export const UnsuccessfulLoginForUnknownUser: Story = {
  args: {
    auditLog: MockAuditLogUnsuccessfulLoginKnownUser,
  },
};
