import type { Meta, StoryObj } from "@storybook/react";
import { CreateTokenPage } from "./CreateTokenPage";

const meta: Meta<typeof CreateTokenPage> = {
  title: "components/CreateTokenPage",
  component: CreateTokenPage,
  parameters: {
    queries: [
      {
        key: ["tokenconfig"],
        data: { max_token_lifetime: 1_000 },
      },
    ],
  },
  decorators: [
    (Story) => {
      Date.now = () => new Date("01/01/2014").getTime();
      return <Story />;
    },
  ],
};

export default meta;
type Story = StoryObj<typeof CreateTokenPage>;

export const Default: Story = {};
