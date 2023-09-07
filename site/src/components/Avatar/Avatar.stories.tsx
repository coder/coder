import type { Meta, StoryObj } from "@storybook/react";
import { Avatar, AvatarIcon } from "./Avatar";
import PauseIcon from "@mui/icons-material/PauseOutlined";

const meta: Meta<typeof Avatar> = {
  title: "components/Avatar",
  component: Avatar,
};

export default meta;
type Story = StoryObj<typeof Avatar>;

export const Letter: Story = {
  args: {
    children: "Coder",
  },
};

export const LetterXL = {
  args: {
    children: "Coder",
    size: "xl",
  },
};

export const LetterDarken = {
  args: {
    children: "Coder",
    colorScheme: "darken",
  },
};

export const Image = {
  args: {
    src: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
  },
};

export const ImageXL = {
  args: {
    src: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
    size: "xl",
  },
};

export const MuiIcon = {
  args: {
    children: <PauseIcon />,
  },
};

export const MuiIconDarken = {
  args: {
    children: <PauseIcon />,
    colorScheme: "darken",
  },
};

export const MuiIconXL = {
  args: {
    children: <PauseIcon />,
    size: "xl",
  },
};

export const AvatarIconDarken = {
  args: {
    children: <AvatarIcon src="/icon/database.svg" />,
    colorScheme: "darken",
  },
};
