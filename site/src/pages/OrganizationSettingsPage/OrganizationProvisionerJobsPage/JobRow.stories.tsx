import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import { Table, TableBody } from "components/Table/Table";
import { MockProvisionerJob } from "testHelpers/entities";
import { daysAgo } from "utils/time";
import { JobRow } from "./JobRow";

const meta: Meta<typeof JobRow> = {
	title: "pages/OrganizationProvisionerJobsPage/JobRow",
	component: JobRow,
	args: {
		job: {
			...MockProvisionerJob,
			created_at: daysAgo(2),
		},
	},
	render: (args) => {
		return (
			<Table>
				<TableBody>
					<JobRow {...args} />
				</TableBody>
			</Table>
		);
	},
};

export default meta;
type Story = StoryObj<typeof JobRow>;

export const Close: Story = {};

export const OpenOnClick: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const showMoreButton = canvas.getByRole("button", { name: /show more/i });

		await userEvent.click(showMoreButton);

		const jobId = canvas.getByText(args.job.id);
		expect(jobId).toBeInTheDocument();
	},
};

export const HideOnClick: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		const showMoreButton = canvas.getByRole("button", { name: /show more/i });
		await userEvent.click(showMoreButton);

		const hideButton = canvas.getByRole("button", { name: /hide/i });
		await userEvent.click(hideButton);

		const jobId = canvas.queryByText(args.job.id);
		expect(jobId).not.toBeInTheDocument();
	},
};
