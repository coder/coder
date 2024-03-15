import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { useState } from "react";
import type { TemplateAutostartRequirementDaysValue } from "utils/schedule";
import { TemplateScheduleAutostart } from "./TemplateScheduleAutostart";

const meta: Meta<typeof TemplateScheduleAutostart> = {
  title: "pages/TemplateSettingsPage/TemplateScheduleAutostart",
  component: TemplateScheduleAutostart,
  args: {
    value: [],
  },
};

export default meta;
type Story = StoryObj<typeof TemplateScheduleAutostart>;

export const AllowAutoStart: Story = {
  args: {
    enabled: true,
  },
  render: function TemplateScheduleAutost(args) {
    const [value, setValue] = useState<TemplateAutostartRequirementDaysValue[]>(
      args.value,
    );
    return (
      <TemplateScheduleAutostart {...args} value={value} onChange={setValue} />
    );
  },
  play: async ({ canvasElement, step }) => {
    const canvas = within(canvasElement);

    await step("select days of week", async () => {
      const daysToSelect = ["Mon", "Tue", "Wed", "Thu", "Fri"];
      for (const day of daysToSelect) {
        await userEvent.click(canvas.getByRole("button", { name: day }));
      }
    });
  },
};

export const DisabledAutoStart: Story = {
  args: {
    enabled: false,
  },
};
