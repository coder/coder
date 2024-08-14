import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableContainer from "@mui/material/TableContainer";
import type { Meta, StoryObj } from "@storybook/react";
import { CodeExample } from "components/CodeExample/CodeExample";
import { TableEmpty } from "./TableEmpty";

const meta: Meta<typeof TableEmpty> = {
  title: "components/TableEmpty",
  component: TableEmpty,
  args: {
    message: "Unfortunately, there's a radio connected to my brain",
  },
  decorators: [
    (Story) => (
      <TableContainer>
        <Table>
          <TableBody>
            <Story />
          </TableBody>
        </Table>
      </TableContainer>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TableEmpty>;

export const Example: Story = {};

export const WithImageAndCta: Story = {
  name: "With Image and CTA",
  args: {
    description: "A gruff voice crackles to life on the intercom.",
    cta: (
      <CodeExample
        secret={false}
        code="say &ldquo;Actually, it's the BBC controlling us from London&rdquo;"
      />
    ),
    image: (
      <img
        src="/featured/templates.webp"
        alt=""
        css={{
          maxWidth: 800,
          height: 320,
          overflow: "hidden",
          objectFit: "cover",
          objectPosition: "top",
        }}
      />
    ),
    style: { paddingBottom: 0 },
  },
};
