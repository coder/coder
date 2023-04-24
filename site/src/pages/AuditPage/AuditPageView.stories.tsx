import { ComponentMeta, Story } from "@storybook/react"
import { createPaginationRef } from "components/PaginationWidget/utils"
import { MockAuditLog, MockAuditLog2 } from "testHelpers/entities"
import { AuditPageView, AuditPageViewProps } from "./AuditPageView"

export default {
  title: "pages/AuditPageView",
  component: AuditPageView,
  args: {
    auditLogs: [MockAuditLog, MockAuditLog2],
    count: 1000,
    paginationRef: createPaginationRef({ page: 1, limit: 25 }),
    isAuditLogVisible: true,
  },
} as ComponentMeta<typeof AuditPageView>

const Template: Story<AuditPageViewProps> = (args) => (
  <AuditPageView {...args} />
)

export const AuditPage = Template.bind({})

export const Loading = Template.bind({})
Loading.args = {
  auditLogs: undefined,
  count: undefined,
  isNonInitialPage: false,
}

export const EmptyPage = Template.bind({})
EmptyPage.args = {
  auditLogs: [],
  isNonInitialPage: true,
}

export const NoLogs = Template.bind({})
NoLogs.args = {
  auditLogs: [],
  count: 0,
  isNonInitialPage: false,
}

export const NotVisible = Template.bind({})
NotVisible.args = {
  isAuditLogVisible: false,
}

export const AuditPageSmallViewport = Template.bind({})
AuditPageSmallViewport.parameters = {
  chromatic: { viewports: [600] },
}
