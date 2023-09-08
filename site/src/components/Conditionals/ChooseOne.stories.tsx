import { Meta, StoryObj } from "@storybook/react";
import { ChooseOne, Cond } from "./ChooseOne";

const meta: Meta<typeof ChooseOne> = {
  title: "components/Conditionals/ChooseOne",
  component: ChooseOne,
};

export default meta;
type Story = StoryObj<typeof ChooseOne>;

export const FirstIsTrue: Story = {
  args: {
    children: (
      <>
        <Cond condition>The first one shows.</Cond>
        <Cond condition={false}>The second one does not show.</Cond>
        <Cond>The default does not show.</Cond>
      </>
    ),
  },
};

export const SecondIsTrue: Story = {
  args: {
    children: (
      <>
        <Cond condition={false}>The first one does not show.</Cond>
        <Cond condition>The second one shows.</Cond>
        <Cond>The default does not show.</Cond>
      </>
    ),
  },
};
export const AllAreTrue: Story = {
  args: {
    children: (
      <>
        <Cond condition>Only the first one shows.</Cond>
        <Cond condition>The second one does not show.</Cond>
        <Cond>The default does not show.</Cond>
      </>
    ),
  },
};

export const NoneAreTrue: Story = {
  args: {
    children: (
      <>
        <Cond condition={false}>The first one does not show.</Cond>
        <Cond condition={false}>The second one does not show.</Cond>
        <Cond>The default shows.</Cond>
      </>
    ),
  },
};

export const OneCond: Story = {
  args: {
    children: <Cond>An only child renders.</Cond>,
  },
};
