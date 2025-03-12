import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import type { ProvisionerJob } from "api/typesGenerated";
import { OrganizationSettingsContext } from "modules/management/OrganizationSettingsLayout";
import {
	MockOrganization,
	MockOrganizationPermissions,
	MockProvisionerJob,
} from "testHelpers/entities";
import { daysAgo } from "utils/time";
import OrganizationProvisionerJobsPage from "./OrganizationProvisionerJobsPage";

const defaultOrganizationSettingsValue = {
	organization: MockOrganization,
	organizationPermissionsByOrganizationId: {},
	organizations: [MockOrganization],
	organizationPermissions: MockOrganizationPermissions,
};

const meta: Meta<typeof OrganizationProvisionerJobsPage> = {
	title: "pages/OrganizationProvisionerJobsPage",
	component: OrganizationProvisionerJobsPage,
	decorators: [
		(Story, { parameters }) => (
			<OrganizationSettingsContext.Provider
				value={parameters.organizationSettingsValue}
			>
				<Story />
			</OrganizationSettingsContext.Provider>
		),
	],
	args: {
		getProvisionerJobs: async () => MockProvisionerJobs,
	},
	parameters: {
		organizationSettingsValue: defaultOrganizationSettingsValue,
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationProvisionerJobsPage>;

export const Default: Story = {};

export const OrganizationNotFound: Story = {
	parameters: {
		organizationSettingsValue: {
			...defaultOrganizationSettingsValue,
			organization: null,
		},
	},
};

export const Loading: Story = {
	args: {
		getProvisionerJobs: () =>
			new Promise((res) => {
				setTimeout(res, 100_000);
			}),
	},
};

export const LoadingError: Story = {
	args: {
		getProvisionerJobs: async () => {
			throw new Error("Failed to load jobs");
		},
	},
};

export const RetryAfterError: Story = {
	args: {
		getProvisionerJobs: (() => {
			let count = 0;

			return async () => {
				count++;

				if (count === 1) {
					throw new Error("Failed to load jobs");
				}

				return MockProvisionerJobs;
			};
		})(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const retryButton = await canvas.findByRole("button", { name: "Retry" });

		userEvent.click(retryButton);

		await waitFor(() => {
			const rows = canvasElement.querySelectorAll("tbody > tr");
			expect(rows).toHaveLength(MockProvisionerJobs.length);
		});
	},
};

export const Empty: Story = {
	args: {
		getProvisionerJobs: async () => [],
	},
};

const MockProvisionerJobs: ProvisionerJob[] = Array.from(
	{ length: 50 },
	(_, i) => ({
		...MockProvisionerJob,
		id: i.toString(),
		created_at: daysAgo(2),
	}),
);
