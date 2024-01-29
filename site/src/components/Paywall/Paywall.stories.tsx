import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import ArrowRightAltOutlined from "@mui/icons-material/ArrowRightAltOutlined";
import type { Meta, StoryObj } from "@storybook/react";
import { Stack } from "../Stack/Stack";
import { Paywall } from "./Paywall";

const meta: Meta<typeof Paywall> = {
  title: "components/Paywall",
  component: Paywall,
};

export default meta;
type Story = StoryObj<typeof Paywall>;

const Example: Story = {
  args: {
    message: "Black Lotus",
    description:
      "Adds 3 mana of any single color of your choice to your mana pool, then is discarded. Tapping this artifact can be played as an interrupt.",
    cta: (
      <Stack direction="row" alignItems="center">
        <Link target="_blank" rel="noreferrer">
          <Button href="#" size="small" startIcon={<ArrowRightAltOutlined />}>
            See how to upgrade
          </Button>
        </Link>
        <Link href="#" target="_blank" rel="noreferrer">
          Read the documentation
        </Link>
      </Stack>
    ),
  },
};

export { Example as Paywall };
