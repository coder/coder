import { Story } from "@storybook/react";
import { Avatar, AvatarIcon, AvatarProps } from "./Avatar";
import PauseIcon from "@mui/icons-material/PauseOutlined";

export default {
  title: "components/Avatar",
  component: Avatar,
};

const Template: Story<AvatarProps> = (args: AvatarProps) => (
  <Avatar {...args} />
);

export const Letter = Template.bind({});
Letter.args = {
  children: "Coder",
};

export const LetterXL = Template.bind({});
LetterXL.args = {
  children: "Coder",
  size: "xl",
};

export const LetterDarken = Template.bind({});
LetterDarken.args = {
  children: "Coder",
  colorScheme: "darken",
};

export const Image = Template.bind({});
Image.args = {
  src: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
};

export const ImageXL = Template.bind({});
ImageXL.args = {
  src: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
  size: "xl",
};

export const MuiIcon = Template.bind({});
MuiIcon.args = {
  children: <PauseIcon />,
};

export const MuiIconDarken = Template.bind({});
MuiIconDarken.args = {
  children: <PauseIcon />,
  colorScheme: "darken",
};

export const MuiIconXL = Template.bind({});
MuiIconXL.args = {
  children: <PauseIcon />,
  size: "xl",
};

export const AvatarIconDarken = Template.bind({});
AvatarIconDarken.args = {
  children: <AvatarIcon src="/icon/database.svg" />,
  colorScheme: "darken",
};
