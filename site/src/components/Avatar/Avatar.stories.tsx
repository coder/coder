import type { Meta, StoryObj } from "@storybook/react";
import { Avatar, AvatarIcon } from "./Avatar";
import PauseIcon from "@mui/icons-material/PauseOutlined";

const meta: Meta<typeof Avatar> = {
  title: "components/Avatar",
  component: Avatar,
};

export default meta;
type Story = StoryObj<typeof Avatar>;

export const WithLetter: Story = {
  args: {
    children: "Coder",
  },
};

export const WithLetterXL = {
  args: {
    children: "Coder",
    size: "xl",
  },
};

export const WithImage = {
  args: {
    src: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
  },
};

export const WithImageXL = {
  args: {
    src: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
    size: "xl",
  },
};

export const WithMuiIcon = {
  args: {
    background: true,
    children: <PauseIcon />,
  },
};

export const WithMuiIconXL = {
  args: {
    background: true,
    children: <PauseIcon />,
    size: "xl",
  },
};

export const WithAvatarIcon = {
  args: {
    background: true,
    children: <AvatarIcon src="/icon/database.svg" alt="Database" />,
  },
};
