import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import {
  MockAuditLog,
  MockAuditLog2,
  MockAuditLogWithWorkspaceBuild,
  MockAuditLogWithDeletedResource,
  MockAuditLogGitSSH,
} from "testHelpers/entities";
import { AuditLogRow } from "./AuditLogRow";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof AuditLogRow> = {
  title: "pages/AuditPage/AuditLogRow",
  component: AuditLogRow,
  decorators: [
    (Story) => (
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell style={{ paddingLeft: 32 }}>Logs</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            <Story />
          </TableBody>
        </Table>
      </TableContainer>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof AuditLogRow>;

export const NoDiff: Story = {
  args: {
    auditLog: {
      ...MockAuditLog,
      diff: {},
    },
  },
};

export const WithDiff: Story = {
  args: {
    auditLog: MockAuditLog2,
    defaultIsDiffOpen: true,
  },
};

export const WithLongDiffRow: Story = {
  args: {
    auditLog: {
      ...MockAuditLog2,
      diff: {
        ...MockAuditLog2.diff,
        icon: {
          old: "https://www.google.com/url?sa=i&url=https%3A%2F%2Fwww.docker.com%2Fcompany%2Fnewsroom%2Fmedia-resources%2F&psig=AOvVaw3hLg_lm0tzXPBt74XZD2GC&ust=1666892413988000&source=images&cd=vfe&ved=0CAwQjRxqFwoTCPDsiKa4_voCFQAAAAAdAAAAABAD",
          new: "https://www.google.com/url?sa=i&url=https%3A%2F%2Fwww.kindpng.com%2Fimgv%2FhRowRxi_docker-icon-png-transparent-png%2F&psig=AOvVaw3hLg_lm0tzXPBt74XZD2GC&ust=1666892413988000&source=images&cd=vfe&ved=0CAwQjRxqFwoTCPDsiKa4_voCFQAAAAAdAAAAABAI",
          secret: false,
        },
      },
    },
    defaultIsDiffOpen: true,
  },
};

export const WithStoppedWorkspaceBuild: Story = {
  args: {
    auditLog: {
      ...MockAuditLogWithWorkspaceBuild,
      action: "stop",
    },
  },
};

export const WithStartedWorkspaceBuild: Story = {
  args: {
    auditLog: {
      ...MockAuditLogWithWorkspaceBuild,
      action: "start",
    },
  },
};

export const WithDeletedWorkspaceBuild: Story = {
  args: {
    auditLog: {
      ...MockAuditLogWithWorkspaceBuild,
      action: "delete",
      is_deleted: true,
    },
  },
};

export const DeletedResource: Story = {
  args: {
    auditLog: MockAuditLogWithDeletedResource,
  },
};

export const SecretDiffValue: Story = {
  args: {
    auditLog: MockAuditLogGitSSH,
  },
};
