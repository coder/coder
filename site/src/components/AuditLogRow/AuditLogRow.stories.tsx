import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
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

export const WithWorkspaceBuild = Template.bind({})
WithWorkspaceBuild.args = {
  auditLog: MockAuditLogWithWorkspaceBuild,
}

export const DeletedResource = Template.bind({})
DeletedResource.args = {
  auditLog: MockAuditLogWithDeletedResource,
}

export const SecretDiffValue = Template.bind({})
SecretDiffValue.args = {
  auditLog: MockAuditLogGitSSH,
}
