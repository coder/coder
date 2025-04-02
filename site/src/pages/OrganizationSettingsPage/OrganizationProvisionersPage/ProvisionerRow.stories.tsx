import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, within } from "@storybook/test";
import { Table, TableBody } from "components/Table/Table";
import { MockBuildInfo, MockProvisioner } from "testHelpers/entities";
import { ProvisionerRow } from "./ProvisionerRow";

const meta: Meta<typeof ProvisionerRow> = {
	title: "pages/OrganizationProvisionersPage/ProvisionerRow",
	component: ProvisionerRow,
	args: {
		provisioner: MockProvisioner,
		buildVersion: MockBuildInfo.version,
	},
	render: (args) => {
		return (
			<Table>
				<TableBody>
					<ProvisionerRow {...args} />
				</TableBody>
			</Table>
		);
	},
};

export default meta;
type Story = StoryObj<typeof ProvisionerRow>;

export const Close: Story = {};

export const Outdated: Story = {
	args: {
		provisioner: {
			...MockProvisioner,
			version: "0.0.0",
		},
	},
};

export const OpenOnClick: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const showMoreButton = canvas.getByRole("button", { name: /show more/i });

		await userEvent.click(showMoreButton);

		const provisionerId = canvas.getByText(args.provisioner.id);
		expect(provisionerId).toBeInTheDocument();
	},
};

export const HideOnClick: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		const showMoreButton = canvas.getByRole("button", { name: /show more/i });
		await userEvent.click(showMoreButton);

		const hideButton = canvas.getByRole("button", { name: /hide/i });
		await userEvent.click(hideButton);

		const provisionerId = canvas.queryByText(args.provisioner.id);
		expect(provisionerId).not.toBeInTheDocument();
	},
};
