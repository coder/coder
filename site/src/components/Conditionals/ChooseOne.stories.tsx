import { Story } from "@storybook/react";
import { ChooseOne, Cond } from "./ChooseOne";

export default {
  title: "components/Conditionals/ChooseOne",
  component: ChooseOne,
  subcomponents: { Cond },
};

export const FirstIsTrue: Story = () => (
  <ChooseOne>
    <Cond condition>The first one shows.</Cond>
    <Cond condition={false}>The second one does not show.</Cond>
    <Cond>The default does not show.</Cond>
  </ChooseOne>
);

export const SecondIsTrue: Story = () => (
  <ChooseOne>
    <Cond condition={false}>The first one does not show.</Cond>
    <Cond condition>The second one shows.</Cond>
    <Cond>The default does not show.</Cond>
  </ChooseOne>
);

export const AllAreTrue: Story = () => (
  <ChooseOne>
    <Cond condition>Only the first one shows.</Cond>
    <Cond condition>The second one does not show.</Cond>
    <Cond>The default does not show.</Cond>
  </ChooseOne>
);

export const NoneAreTrue: Story = () => (
  <ChooseOne>
    <Cond condition={false}>The first one does not show.</Cond>
    <Cond condition={false}>The second one does not show.</Cond>
    <Cond>The default shows.</Cond>
  </ChooseOne>
);

export const OneCond: Story = () => (
  <ChooseOne>
    <Cond>An only child renders.</Cond>
  </ChooseOne>
);
