import { ComponentMeta, Story } from "@storybook/react"
import { MockAuditLog, MockAuditLog2 } from "testHelpers/entities"
import { AuditPageView, AuditPageViewProps } from "./AuditPageView"

export default {
  title: "pages/AuditPageView",
  component: AuditPageView,
} as ComponentMeta<typeof AuditPageView>

const Template: Story<AuditPageViewProps> = (args) => <AuditPageView {...args} />

export const AuditPage = Template.bind({})
AuditPage.args = {
  auditLogs: [MockAuditLog, MockAuditLog2],
  count: 1000,
  page: 1,
  limit: 25,
}

export const AuditPageSmallViewport = Template.bind({})
AuditPageSmallViewport.args = {
  auditLogs: [MockAuditLog, MockAuditLog2],
  count: 1000,
  page: 1,
  limit: 25,
}
AuditPageSmallViewport.parameters = {
  chromatic: { viewports: [600] },
}
