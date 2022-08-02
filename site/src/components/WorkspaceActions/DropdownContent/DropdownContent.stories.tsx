import { Story } from "@storybook/react"
import { DeleteButton, StartButton, StopButton } from "../ActionCtas"
import {
  ButtonMapping,
  ButtonTypesEnum,
  WorkspaceStateActions,
  WorkspaceStateEnum,
} from "../constants"
import { DropdownContent, DropdownContentProps } from "./DropdownContent"

// These are the stories for the secondary actions (housed in the dropdown)
// in WorkspaceActions.tsx

export default {
  title: "WorkspaceActionsDropdown",
  component: DropdownContent,
}

const Template: Story<DropdownContentProps> = (args) => <DropdownContent {...args} />

const buttonMappingMock: Partial<ButtonMapping> = {
  [ButtonTypesEnum.delete]: <DeleteButton handleAction={() => jest.fn()} />,
  [ButtonTypesEnum.start]: <StartButton handleAction={() => jest.fn()} />,
  [ButtonTypesEnum.stop]: <StopButton handleAction={() => jest.fn()} />,
  [ButtonTypesEnum.delete]: <DeleteButton handleAction={() => jest.fn()} />,
}

const defaultArgs = {
  buttonMapping: buttonMappingMock,
}

export const Started = Template.bind({})
Started.args = {
  ...defaultArgs,
  secondaryActions: WorkspaceStateActions[WorkspaceStateEnum.started].secondary,
}

export const Stopped = Template.bind({})
Stopped.args = {
  ...defaultArgs,
  secondaryActions: WorkspaceStateActions[WorkspaceStateEnum.stopped].secondary,
}

export const Canceled = Template.bind({})
Canceled.args = {
  ...defaultArgs,
  secondaryActions: WorkspaceStateActions[WorkspaceStateEnum.canceled].secondary,
}

export const Errored = Template.bind({})
Errored.args = {
  ...defaultArgs,
  secondaryActions: WorkspaceStateActions[WorkspaceStateEnum.error].secondary,
}
