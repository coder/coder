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
    children: [
      <Cond key="1" condition>
        The first one shows.
      </Cond>,
      <Cond key="2" condition={false}>
        The second one does not show.
      </Cond>,
      <Cond key="3">The default does not show.</Cond>,
    ],
  },
};

export const SecondIsTrue: Story = {
  args: {
    children: [
      <Cond key="1" condition={false}>
        The first one does not show.
      </Cond>,
      <Cond key="2" condition>
        The second one shows.
      </Cond>,
      <Cond key="3">The default does not show.</Cond>,
    ],
  },
};
export const AllAreTrue: Story = {
  args: {
    children: [
      <Cond key="1" condition>
        Only the first one shows.
      </Cond>,
      <Cond key="2" condition>
        The second one does not show.
      </Cond>,
      <Cond key="3">The default does not show.</Cond>,
    ],
  },
};

export const NoneAreTrue: Story = {
  args: {
    children: [
      <Cond key="1" condition={false}>
        The first one does not show.
      </Cond>,
      <Cond key="2" condition={false}>
        The second one does not show.
      </Cond>,
      <Cond key="3">The default shows.</Cond>,
    ],
  },
};

export const OneCond: Story = {
  args: {
    children: <Cond>An only child renders.</Cond>,
  },
};
