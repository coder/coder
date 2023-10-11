import { Meta, StoryObj } from "@storybook/react";
import { PopoverContainer } from "./PopoverContainer";
import Button from "@mui/material/Button";

const numbers: number[] = [];
for (let i = 0; i < 20; i++) {
  numbers.push(i + 1);
}

const meta: Meta<typeof PopoverContainer> = {
  title: "components/PopoverContainer",
  component: PopoverContainer,
  args: {
    anchorButton: <Button>I have no hooks/refs</Button>,
    children: <p>Hiya!</p>,
    originY: "bottom",
  },
};

export default meta;

type Story = StoryObj<typeof PopoverContainer>;
const Example: Story = {};

export { Example as PopoverContainer };
