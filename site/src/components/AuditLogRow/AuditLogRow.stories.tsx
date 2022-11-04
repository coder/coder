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
        old: "https://www.docker.com/wp-content/uploads/2022/03/Moby-logo.png",
        new: "https://www.docker.com/wp-content/uploads/2022/03/vertical-logo-monochromatic.png",
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
