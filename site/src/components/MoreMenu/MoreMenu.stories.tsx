import GrassIcon from "@mui/icons-material/Grass";
import KitesurfingIcon from "@mui/icons-material/Kitesurfing";
import type { Meta, StoryObj } from "@storybook/react";
import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
  ThreeDotsButton,
} from "./MoreMenu";
import { action } from "@storybook/addon-actions";
import { expect, screen, userEvent, waitFor, within } from "@storybook/test";

const meta: Meta<typeof MoreMenu> = {
  title: "components/MoreMenu",
  component: MoreMenu,
};

export default meta;
type Story = StoryObj<typeof MoreMenu>;

const Example: Story = {
  args: {
    children: (
      <>
        <MoreMenuTrigger>
          <ThreeDotsButton />
        </MoreMenuTrigger>
        <MoreMenuContent>
          <MoreMenuItem onClick={() => action("grass")}>
            <GrassIcon />
            Touch grass
          </MoreMenuItem>
          <MoreMenuItem onClick={() => action("water")}>
            <KitesurfingIcon />
            Touch water
          </MoreMenuItem>
        </MoreMenuContent>
      </>
    ),
  },
  play: async ({ canvasElement, step }) => {
    const canvas = within(canvasElement);

    await step("Open menu", async () => {
      await userEvent.click(canvas.getByRole("button"));
      await waitFor(() => {
        expect(screen.getByText(/touch grass/i)).toBeInTheDocument();
        expect(screen.getByText(/touch water/i)).toBeInTheDocument();
      });
    });
  },
};

export { Example as MoreMenu };
