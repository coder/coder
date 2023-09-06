import { Story } from "@storybook/react";
import { AvatarData, AvatarDataProps } from "./AvatarData";

export default {
  title: "components/AvatarData",
  component: AvatarData,
};

const Template: Story<AvatarDataProps> = (args: AvatarDataProps) => (
  <AvatarData {...args} />
);

export const Example = Template.bind({});
Example.args = {
  title: "coder",
  subtitle: "coder@coder.com",
};

export const WithImage = Template.bind({});
WithImage.args = {
  title: "coder",
  subtitle: "coder@coder.com",
  src: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
};
