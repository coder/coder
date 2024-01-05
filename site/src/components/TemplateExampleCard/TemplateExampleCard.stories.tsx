import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import {
  MockTemplateExample,
  MockTemplateExample2,
} from "testHelpers/entities";
import { TemplateExampleCard } from "./TemplateExampleCard";

const meta: Meta<typeof TemplateExampleCard> = {
  title: "components/TemplateExampleCard",
  parameters: { chromatic },
  component: TemplateExampleCard,
  args: {
    example: MockTemplateExample,
  },
};

export default meta;
type Story = StoryObj<typeof TemplateExampleCard>;

export const Example: Story = {};

export const ByTag: Story = {
  args: {
    activeTag: "cloud",
  },
};

export const LotsOfTags: Story = {
  args: {
    example: {
      ...MockTemplateExample2,
      tags: ["omg", "so many tags", "look at all these", "so cool"],
    },
  },
};
