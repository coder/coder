import { Story } from "@storybook/react"
import { ChooseOne, Cond } from "./ChooseOne"

export default {
  title: "components/Conditionals/ChooseOne",
  component: ChooseOne,
  subcomponents: { Cond },
}

export const FirstIsTrue: Story = () => (
  <ChooseOne>
    <Cond condition>The first one shows.</Cond>
    <Cond condition={false}>The second one does not show.</Cond>
  </ChooseOne>
)

export const SecondIsTrue: Story = () => (
  <ChooseOne>
    <Cond condition={false}>The first one does not show.</Cond>
    <Cond condition>The second one shows.</Cond>
  </ChooseOne>
)

export const AllAreTrue: Story = () => (
  <ChooseOne>
    <Cond condition>Only the first one shows.</Cond>
    <Cond condition>The second one does not show.</Cond>
  </ChooseOne>
)

export const NoneAreTrue: Story = () => (
  <ChooseOne>
    <Cond condition={false}>The first one does not show.</Cond>
    <Cond condition={false}>The second shows because it is the fallback.</Cond>
  </ChooseOne>
)
