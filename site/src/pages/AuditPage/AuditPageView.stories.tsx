import { ComponentMeta, Story } from "@storybook/react"
import { MockAuditLog, MockAuditLogWithDiff } from "testHelpers/entities"
import { AuditPageView } from "./AuditPageView"

export default {
  title: "pages/AuditPageView",
  component: AuditPageView,
} as ComponentMeta<typeof AuditPageView>

const Template: Story = (args) => <AuditPageView {...args} />

export const AuditPage = Template.bind({})
AuditPage.args = {
  auditLogs: [MockAuditLog, MockAuditLog, MockAuditLogWithDiff],
}

export const AuditPageSmallViewport = Template.bind({})
AuditPageSmallViewport.parameters = {
  chromatic: { viewports: [600] },
}
