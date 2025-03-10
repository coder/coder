import type { Meta, StoryObj } from "@storybook/react";
import { daysAgo } from "utils/time";
import { userEvent, waitFor, within, expect } from "@storybook/test";
import { JobRow } from "./JobRow";
import { Table, TableBody } from "components/Table/Table";

const meta: Meta<typeof JobRow> = {
	title: "pages/OrganizationProvisionerJobsPage/JobRow",
	component: JobRow,
	args: {
		job: {
			id: "373cb47a-c33b-4e35-83bf-23c64fde0293",
			created_at: daysAgo(2),
			started_at: "2025-03-09T08:01:22.118677Z",
			completed_at: "2025-03-09T08:01:33.310184Z",
			status: "succeeded",
			worker_id: "d4f5452e-3d1b-4400-a7c1-233c2dee6f1a",
			file_id: "a99685a6-4441-4258-8102-f07e571204fb",
			tags: {
				cluster: "dogfood-v2",
				env: "gke",
				owner: "",
				scope: "organization",
			},
			queue_position: 0,
			queue_size: 24,
			organization_id: "703f72a1-76f6-4f89-9de6-8a3989693fe5",
			input: {
				workspace_build_id: "0153584a-6cbe-4c90-8e9e-4ffb40fd1069",
			},
			type: "workspace_build",
			metadata: {
				template_version_name: "54745b1",
				template_id: "0d286645-29aa-4eaf-9b52-cc5d2740c90b",
				template_name: "coder",
				template_display_name: "Write Coder on Coder",
				template_icon: "/emojis/1f3c5.png",
				workspace_id: "e1a7f977-28e7-4567-b91a-179a14c3658b",
				workspace_name: "nowjosias",
			},
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

export const Open: Story = {
	args: {
		defaultOpen: true,
	},
};

export const OpenOnClick: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const showMoreButton = canvas.getByRole("button", { name: /show more/i });

		await userEvent.click(showMoreButton);

		const jobId = canvas.getByText(args.job.id);
		expect(jobId).toBeInTheDocument();
	},
	parameters: {
		chromatic: {
			disableSnapshot: true,
		},
	},
};

export const HideOnClick: Story = {
	args: {
		defaultOpen: true,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const showMoreButton = canvas.getByRole("button", { name: /hide/i });

		await userEvent.click(showMoreButton);

		const jobId = canvas.queryByText(args.job.id);
		expect(jobId).not.toBeInTheDocument();
	},
	parameters: {
		chromatic: {
			disableSnapshot: true,
		},
	},
};
