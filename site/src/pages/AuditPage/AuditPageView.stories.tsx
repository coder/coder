import { ComponentMeta, Story } from "@storybook/react"
import { createPaginationRef } from "components/PaginationWidget/utils"
import { MockAuditLog, MockAuditLog2 } from "testHelpers/entities"
import { AuditPageView, AuditPageViewProps } from "./AuditPageView"

export default {
  title: "pages/AuditPageView",
  component: AuditPageView,
  argTypes: {
    auditLogs: {
      defaultValue: [MockAuditLog, MockAuditLog2],
    },
    count: {
      defaultValue: 1000,
    },
    paginationRef: {
      defaultValue: createPaginationRef({ page: 1, limit: 25 }),
    },
  },
} as ComponentMeta<typeof AuditPageView>

const Template: Story<AuditPageViewProps> = (args) => (
  <AuditPageView {...args} />
)

export const AuditPage = Template.bind({})

export const AuditPageSmallViewport = Template.bind({})
AuditPageSmallViewport.parameters = {
  chromatic: { viewports: [600] },
}
