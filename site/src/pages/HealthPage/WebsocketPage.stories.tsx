import type { StoryObj, Meta } from "@storybook/react";
import { generateMeta } from "./storybook";
import { WebsocketPage } from "./WebsocketPage";

const meta: Meta = {
  title: "pages/Health/Websocket",
  ...generateMeta({
    path: "/health/websocket",
    element: <WebsocketPage />,
  }),
};

export default meta;
type Story = StoryObj;

const Example: Story = {};

export { Example as Websocket };
