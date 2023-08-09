import type { Meta, StoryObj } from "@storybook/react"
import { InactivityDialog } from "./InactivityDialog"

const meta: Meta<typeof InactivityDialog> = {
  title: "InactivityDialog",
  component: InactivityDialog,
}

export default meta
type Story = StoryObj<typeof InactivityDialog>

export const OpenDialog: Story = {
  args: {
    submitValues: () => null,
    isInactivityDialogOpen: true,
    setIsInactivityDialogOpen: () => null,
    workspacesToBeLockedToday: 2,
  },
}
