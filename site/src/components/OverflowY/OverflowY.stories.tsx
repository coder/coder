import type { Meta, StoryObj } from "@storybook/react";
import { OverflowY } from "./OverflowY";

const numbers: number[] = [];
for (let i = 0; i < 20; i++) {
  numbers.push(i + 1);
}

const meta: Meta<typeof OverflowY> = {
  title: "components/OverflowY",
  component: OverflowY,
  args: {
    maxHeight: 400,
    children: numbers.map((num, i) => (
      <p
        key={num}
        css={{
          height: "50px",
          padding: 0,
          margin: 0,
          color: "black",
          backgroundColor: i % 2 === 0 ? "white" : "gray",
        }}
      >
        Element {num}
      </p>
    )),
  },
};

export default meta;

type Story = StoryObj<typeof OverflowY>;
const Example: Story = {};

export { Example as OverflowY };
