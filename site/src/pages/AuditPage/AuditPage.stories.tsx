import {
	MockAuditLog,
	MockAuditLog2,
	MockEntitlementsWithAuditLog,
	MockUserOwner,
} from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import AuditPage from "./AuditPage";

const ownerAutocompleteQuery = {
	key: ["owner", "autocomplete", "search", ""],
	data: [],
};

const auditLogsQuery = (
	filter: string,
	pageNumber: number,
	auditLogs: [typeof MockAuditLog, ...(typeof MockAuditLog)[]],
	count: number,
) => ({
	key: ["auditLogs", filter, pageNumber],
	data: {
		audit_logs: auditLogs,
		count,
	},
});

const meta: Meta<typeof AuditPage> = {
	title: "pages/AuditPage/InteractionTests",
	component: AuditPage,
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
	parameters: {
		user: MockUserOwner,
		features: MockEntitlementsWithAuditLog.features.audit_log.enabled
			? ["audit_log"]
			: [],
		queries: [ownerAutocompleteQuery],
	},
};

export default meta;
type Story = StoryObj<typeof AuditPage>;

export const PageFive: Story = {
	parameters: {
		queries: [
			ownerAutocompleteQuery,
			auditLogsQuery("", 5, [MockAuditLog, MockAuditLog2], 200),
		],
		reactRouter: reactRouterParameters({
			location: { searchParams: { page: "5" } },
		}),
	},
	play: async () => {
		await screen.findByTestId(`audit-log-row-${MockAuditLog.id}`);
		expect(
			screen.getByTestId(`audit-log-row-${MockAuditLog.id}`),
		).toBeInTheDocument();
		expect(
			screen.getByTestId(`audit-log-row-${MockAuditLog2.id}`),
		).toBeInTheDocument();
	},
};

export const ExpandableRowWithEnter: Story = {
	parameters: {
		queries: [ownerAutocompleteQuery, auditLogsQuery("", 1, [MockAuditLog], 1)],
	},
	play: async () => {
		const user = userEvent.setup();
		const row = await screen.findByTestId(`audit-log-row-${MockAuditLog.id}`);
		const expandableRowButton = within(row).getByRole("button");

		expect(screen.queryByText(/ttl:/i)).not.toBeInTheDocument();

		expandableRowButton.focus();
		await user.keyboard("{Enter}");

		expect(screen.getAllByText(/ttl:/i)).toHaveLength(2);

		expandableRowButton.focus();
		await user.keyboard("{Enter}");

		await waitFor(() => {
			expect(screen.queryByText(/ttl:/i)).not.toBeInTheDocument();
		});
	},
};

export const ExpandableRowWithSpace: Story = {
	parameters: {
		queries: [ownerAutocompleteQuery, auditLogsQuery("", 1, [MockAuditLog], 1)],
	},
	play: async () => {
		const user = userEvent.setup();
		const row = await screen.findByTestId(`audit-log-row-${MockAuditLog.id}`);
		const expandableRowButton = within(row).getByRole("button");

		expect(screen.queryByText(/ttl:/i)).not.toBeInTheDocument();

		expandableRowButton.focus();
		await user.keyboard(" ");

		expect(screen.getAllByText(/ttl:/i)).toHaveLength(2);

		expandableRowButton.focus();
		await user.keyboard(" ");

		await waitFor(() => {
			expect(screen.queryByText(/ttl:/i)).not.toBeInTheDocument();
		});
	},
};

export const FilteredFromURL: Story = {
	parameters: {
		queries: [
			ownerAutocompleteQuery,
			auditLogsQuery(
				"resource_type:workspace action:create",
				1,
				[MockAuditLog],
				1,
			),
		],
		reactRouter: reactRouterParameters({
			location: {
				searchParams: {
					filter: "resource_type:workspace action:create",
				},
			},
		}),
	},
	play: async () => {
		const filterInput = await screen.findByLabelText("Filter");

		expect(
			screen.getByTestId(`audit-log-row-${MockAuditLog.id}`),
		).toBeInTheDocument();
		expect(filterInput).toHaveValue("resource_type:workspace action:create");
	},
};

export const FilterChangeResetsToFirstPage: Story = {
	parameters: {
		queries: [
			ownerAutocompleteQuery,
			auditLogsQuery("", 2, [MockAuditLog], 60),
			auditLogsQuery("", 1, [MockAuditLog], 60),
			auditLogsQuery(
				"resource_type:workspace action:create",
				1,
				[MockAuditLog],
				1,
			),
		],
		reactRouter: reactRouterParameters({
			location: { searchParams: { page: "2" } },
		}),
	},
	play: async () => {
		const user = userEvent.setup();
		await screen.findByTestId(`audit-log-row-${MockAuditLog.id}`);

		const filterInput = screen.getByLabelText("Filter");
		const query = "resource_type:workspace action:create";
		await user.type(filterInput, query);

		await waitFor(
			() => {
				expect(filterInput).toHaveValue(query);
				expect(
					screen.getByTestId(`audit-log-row-${MockAuditLog.id}`),
				).toBeInTheDocument();
			},
			{ timeout: 5000 },
		);
	},
};
