import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableContainer from "@mui/material/TableContainer";
import type { Meta, StoryObj } from "@storybook/react";
import { TableLoader } from "./TableLoader";

const meta: Meta<typeof TableLoader> = {
  title: "components/TableLoader",
  component: TableLoader,
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
type Story = StoryObj<typeof TableLoader>;

export const Example: Story = {};
export { Example as TableLoader };
