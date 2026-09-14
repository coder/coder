import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { MemoryRouter } from "react-router";
import { expect, it, vi } from "vitest";
import { MockMenu } from "#/components/Filter/storyHelpers";
import { mockSuccessResult } from "#/components/PaginationWidget/PaginationContainer.mocks";
import {
	MockAIGatewaySpendUser,
	MockAIGatewaySpendUserSummary,
	MockUserMember,
} from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { SpendPageView } from "./SpendPageView";

type Props = ComponentProps<typeof SpendPageView>;

function renderView(overrides: Partial<Props> = {}) {
	const props: Props = {
		isEntitled: true,
		isEnabled: true,
		dateRange: {
			startDate: new Date("2026-02-10T00:00:00Z"),
			endDate: new Date("2026-03-12T00:00:00Z"),
		},
		dimensions: {},
		filterMenus: { provider: MockMenu, client: MockMenu, model: MockMenu },
		searchFilter: "",
		usersQuery: {
			...mockSuccessResult,
			totalRecords: 1,
			data: {
				start_date: "2026-02-10T00:00:00Z",
				end_date: "2026-03-12T00:00:00Z",
				count: 1,
				users: [MockAIGatewaySpendUser],
			},
			isLoading: false,
			isFetching: false,
			error: undefined,
			refetch: vi.fn(),
		},
		drillInUserId: null,
		drillInUser: null,
		isDrillInUserLoading: false,
		drillInUserError: undefined,
		summaryData: undefined,
		isSummaryLoading: false,
		summaryError: undefined,
		onDateRangeChange: vi.fn(),
		onSearchFilterChange: vi.fn(),
		onDrillInUserRetry: vi.fn(),
		onClearSelectedUser: vi.fn(),
		onSummaryRetry: vi.fn(),
		...overrides,
	};
	const view = renderComponent(
		<MemoryRouter>
			<SpendPageView {...props} />
		</MemoryRouter>,
	);
	return { ...view, props };
}

it("reports the search draft", async () => {
	const user = userEvent.setup();
	const { props } = renderView();
	await user.type(
		screen.getByRole("textbox", { name: "Search spend by name or username" }),
		" bob ",
	);
	expect(props.onSearchFilterChange).toHaveBeenLastCalledWith(" bob ");
});

it.each([false, true])(
	"retries the users query with retained data: %s",
	async (hasData) => {
		const user = userEvent.setup();
		const { props, rerender } = renderView();
		rerender(
			<MemoryRouter>
				<SpendPageView
					{...props}
					usersQuery={{
						...props.usersQuery,
						data: hasData ? props.usersQuery.data : undefined,
						error: new Error("Unable to load users"),
					}}
				/>
			</MemoryRouter>,
		);
		await user.click(screen.getByRole("button", { name: "Retry" }));
		expect(props.usersQuery.refetch).toHaveBeenCalledOnce();
	},
);

it.each([
	{ drillInUserId: null, hasData: false },
	{ drillInUserId: null, hasData: true },
	{ drillInUserId: MockAIGatewaySpendUser.id, hasData: false },
])(
	"retries the summary for $drillInUserId with retained data: $hasData",
	async ({ drillInUserId, hasData }) => {
		const user = userEvent.setup();
		const { props } = renderView({
			drillInUserId,
			drillInUser: MockUserMember,
			summaryError: new Error("Unable to load spend"),
			summaryData: hasData ? MockAIGatewaySpendUserSummary : undefined,
		});
		await user.click(screen.getByRole("button", { name: "Retry" }));
		expect(props.onSummaryRetry).toHaveBeenCalledOnce();
	},
);

it("retries a failed user lookup", async () => {
	const user = userEvent.setup();
	const { props } = renderView({
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUserError: new Error("User not found"),
	});
	await user.click(screen.getByRole("button", { name: "Retry" }));
	expect(props.onDrillInUserRetry).toHaveBeenCalledOnce();
});

it("clears the selected user on Back", async () => {
	const user = userEvent.setup();
	const { props } = renderView({
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: MockUserMember,
		summaryData: MockAIGatewaySpendUserSummary,
	});
	await user.click(screen.getByRole("button", { name: "Back" }));
	expect(props.onClearSelectedUser).toHaveBeenCalledOnce();
});

it.each([
	{ dimensions: {}, start: "2026-02-10T00:00:00Z", filter: "" },
	{
		dimensions: { provider_name: "anthropic-main", client: "Claude Code" },
		start: "2026-02-10T00:00:00Z",
		filter: 'provider_name:anthropic-main client:"Claude Code" ',
	},
	{ dimensions: {}, start: "2026-02-26T12:00:00Z", filter: "" },
])(
	"links to sessions with applied start $start and filters $filter",
	({ dimensions, start, filter }) => {
		renderView({
			drillInUserId: MockAIGatewaySpendUser.id,
			drillInUser: { ...MockUserMember, id: MockAIGatewaySpendUser.id },
			dimensions,
			summaryData: { ...MockAIGatewaySpendUserSummary, start_date: start },
		});
		const url = new URL(
			screen
				.getByRole("link", { name: "View sessions" })
				.getAttribute("href") ?? "",
			"http://localhost",
		);
		expect(url.pathname).toBe("/ai-gateway/sessions");
		expect(url.searchParams.get("filter")).toBe(
			filter +
				"initiator:" +
				MockAIGatewaySpendUser.id +
				' started_after:"' +
				start +
				'" started_before:"2026-03-12T00:00:00Z"',
		);
	},
);
