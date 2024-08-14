import type { Meta, StoryObj } from "@storybook/react";
import { AvatarCard } from "./AvatarCard";

const meta: Meta<typeof AvatarCard> = {
  title: "components/AvatarCard",
  component: AvatarCard,
};

export default meta;
type Story = StoryObj<typeof AvatarCard>;

export const WithImage: Story = {
  args: {
    header: "Coder",
    imgUrl: "https://avatars.githubusercontent.com/u/95932066?s=200&v=4",
    altText: "Coder",
    subtitle: "56 members",
  },
};

export const WithoutImage: Story = {
  args: {
    header: "Patrick Star",
    subtitle: "Friends with 723 people",
  },
};

export const WithoutSubtitleOrImage: Story = {
  args: {
    header: "Sandy Cheeks",
  },
};
