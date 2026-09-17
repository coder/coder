import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import type { UseFilterResult } from "#/components/Filter/Filter";
import {
	MockStoppedWorkspace,
	MockTemplate,
	MockWorkspace,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { WorkspacesPageView } from "./WorkspacesPageView";

const createFilter = (used = false): UseFilterResult => ({
	query: "",
	values: {},
	used,
	update: vi.fn(),
	debounceUpdate: vi.fn(),
	cancelDebounce: vi.fn(),
});

const defaultProps = {
	error: undefined,
	workspaces: [],
	checkedWorkspaces: [],
	count: 0,
	filter: createFilter(),
	page: 1,
	limit: 25,
	onPageChange: vi.fn(),
	onCheckChange: vi.fn(),
	isRunningBatchAction: false,
	onBatchDeleteTransition: vi.fn(),
	onBatchUpdateTransition: vi.fn(),
	onBatchStartTransition: vi.fn(),
	onBatchStopTransition: vi.fn(),
	templatesFetchStatus: "success" as const,
	templates: [MockTemplate],
	canCreateTemplate: false,
	canCreateWorkspace: true,
	canChangeVersions: false,
	onActionSuccess: vi.fn().mockResolvedValue(undefined),
	onActionError: vi.fn(),
};

describe("WorkspacesPageView", () => {
	it("hides the New workspace button and explains the missing permission", async () => {
		renderWithAuth(
			<WorkspacesPageView {...defaultProps} canCreateWorkspace={false} />,
		);

		await screen.findByText(/don't have permission to create workspaces/i);
		expect(
			screen.queryByRole("button", { name: /new workspace/i }),
		).not.toBeInTheDocument();
	});

	it("shows the New workspace button when the user can create workspaces", async () => {
		renderWithAuth(<WorkspacesPageView {...defaultProps} />);

		expect(
			await screen.findByRole("button", { name: /new workspace/i }),
		).toBeInTheDocument();
	});

	it("shows the filter empty state instead of the no-permission empty state when a filter is active", async () => {
		renderWithAuth(
			<WorkspacesPageView
				{...defaultProps}
				canCreateWorkspace={false}
				filter={createFilter(true)}
			/>,
		);

		await screen.findByText(/no results matched your search/i);
		expect(
			screen.queryByText(/don't have permission to create workspaces/i),
		).not.toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: /new workspace/i }),
		).not.toBeInTheDocument();
	});

	it("shows the filter empty state when the user can create workspaces but the filter matches nothing", async () => {
		renderWithAuth(
			<WorkspacesPageView {...defaultProps} filter={createFilter(true)} />,
		);

		await screen.findByText(/no results matched your search/i);
		expect(
			screen.queryByText(/don't have permission to create workspaces/i),
		).not.toBeInTheDocument();
	});

	it("restarts a running workspace from the overflow menu", async () => {
		const user = userEvent.setup();
		const restartSpy = vi
			.spyOn(API, "restartWorkspace")
			.mockResolvedValue(undefined);

		renderWithAuth(
			<WorkspacesPageView
				{...defaultProps}
				workspaces={[MockWorkspace]}
				count={1}
			/>,
		);

		await screen.findByText(MockWorkspace.name);
		await user.click(screen.getByTestId("workspace-options-button"));

		const restartItem = await screen.findByRole("menuitem", {
			name: /restart/i,
		});
		await user.click(restartItem);

		const dialog = await screen.findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: /restart/i }));

		await waitFor(() =>
			expect(restartSpy).toHaveBeenCalledWith(
				expect.objectContaining({ workspace: MockWorkspace }),
			),
		);
	});

	it("does not offer restart for a stopped workspace", async () => {
		const user = userEvent.setup();

		renderWithAuth(
			<WorkspacesPageView
				{...defaultProps}
				workspaces={[MockStoppedWorkspace]}
				count={1}
			/>,
		);

		await screen.findByText(MockStoppedWorkspace.name);
		await user.click(screen.getByTestId("workspace-options-button"));

		await screen.findByRole("menuitem", { name: /settings/i });
		expect(
			screen.queryByRole("menuitem", { name: /restart/i }),
		).not.toBeInTheDocument();
	});
});
