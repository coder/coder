import { screen } from "@testing-library/react";
import { MockTemplate } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import type { WorkspaceFilterState } from "./filter/WorkspacesFilter";
import { WorkspacesPageView } from "./WorkspacesPageView";

const mockMenu = {
	initialOption: undefined,
	isInitializing: false,
	isSearching: false,
	query: "",
	searchOptions: [],
	selectedOption: undefined,
	selectOption: vi.fn(),
	setQuery: vi.fn(),
};

const createFilterState = (used = false) =>
	({
		filter: {
			query: "",
			values: {},
			used,
			update: vi.fn(),
			debounceUpdate: vi.fn(),
			cancelDebounce: vi.fn(),
		},
		menus: {
			user: mockMenu,
			template: mockMenu,
			status: mockMenu,
			organizations: mockMenu,
		},
	}) as WorkspaceFilterState;

const defaultProps = {
	error: undefined,
	workspaces: [],
	checkedWorkspaces: [],
	count: 0,
	filterState: createFilterState(),
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
				filterState={createFilterState(true)}
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
			<WorkspacesPageView
				{...defaultProps}
				filterState={createFilterState(true)}
			/>,
		);

		await screen.findByText(/no results matched your search/i);
		expect(
			screen.queryByText(/don't have permission to create workspaces/i),
		).not.toBeInTheDocument();
	});
});
