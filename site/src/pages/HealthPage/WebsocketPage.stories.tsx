import { StoryObj, Meta } from "@storybook/react";
import { WebsocketPage } from "./WebsocketPage";
import { generateMeta } from "./storybook";

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
