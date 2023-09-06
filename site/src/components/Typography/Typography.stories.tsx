import { Story } from "@storybook/react";
import { Typography, TypographyProps } from "./Typography";

export default {
  title: "components/Typography",
  component: Typography,
};

const Template: Story<TypographyProps> = (args: TypographyProps) => (
  <>
    <Typography {...args}>Colorless green ideas sleep furiously</Typography>
    <Typography {...args}>
      More people have been to France than I have
    </Typography>
  </>
);

export const Short = Template.bind({});
Short.args = {
  short: true,
};
export const Tall = Template.bind({});
Tall.args = {
  short: false,
};
