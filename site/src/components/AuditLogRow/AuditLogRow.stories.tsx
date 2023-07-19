import Table from "@mui/material/Table"
import TableBody from "@mui/material/TableBody"
import TableCell from "@mui/material/TableCell"
import TableContainer from "@mui/material/TableContainer"
import TableHead from "@mui/material/TableHead"
import TableRow from "@mui/material/TableRow"
import { ComponentMeta, Story } from "@storybook/react"
import {
  MockAuditLog,
  MockAuditLog2,
  MockAuditLogWithWorkspaceBuild,
  MockAuditLogWithDeletedResource,
  MockAuditLogGitSSH,
} from "testHelpers/entities"
import { AuditLogRow, AuditLogRowProps } from "./AuditLogRow"

export default {
  title: "components/AuditLogRow",
  component: AuditLogRow,
} as ComponentMeta<typeof AuditLogRow>

const Template: Story<AuditLogRowProps> = (args) => (
  <TableContainer>
    <Table>
      <TableHead>
        <TableRow>
          <TableCell style={{ paddingLeft: 32 }}>Logs</TableCell>
        </TableRow>
      </TableHead>
      <TableBody>
        <AuditLogRow {...args} />
      </TableBody>
    </Table>
  </TableContainer>
)

export const NoDiff = Template.bind({})
NoDiff.args = {
  auditLog: {
    ...MockAuditLog,
    diff: {},
  },
}

export const WithDiff = Template.bind({})
WithDiff.args = {
  auditLog: MockAuditLog2,
  defaultIsDiffOpen: true,
}

export const WithLongDiffRow = Template.bind({})
WithLongDiffRow.args = {
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
}

export const WithStoppedWorkspaceBuild = Template.bind({})
WithStoppedWorkspaceBuild.args = {
  auditLog: {
    ...MockAuditLogWithWorkspaceBuild,
    action: "stop",
  },
}

export const WithStartedWorkspaceBuild = Template.bind({})
WithStartedWorkspaceBuild.args = {
  auditLog: {
    ...MockAuditLogWithWorkspaceBuild,
    action: "start",
  },
}

export const WithDeletedWorkspaceBuild = Template.bind({})
WithDeletedWorkspaceBuild.args = {
  auditLog: {
    ...MockAuditLogWithWorkspaceBuild,
    action: "delete",
    is_deleted: true,
  },
}

export const DeletedResource = Template.bind({})
DeletedResource.args = {
  auditLog: MockAuditLogWithDeletedResource,
}

export const SecretDiffValue = Template.bind({})
SecretDiffValue.args = {
  auditLog: MockAuditLogGitSSH,
}
