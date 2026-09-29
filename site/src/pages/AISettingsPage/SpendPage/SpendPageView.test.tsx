import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { mockInitialRenderResult } from "#/components/PaginationWidget/PaginationContainer.mocks";
import { MockOrganization, MockOrganization2 } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import type { SpendReportQuery } from "./components/SpendUsersTable";
import { SpendPageView } from "./SpendPageView";

const pendingReportQuery = {
	...mockInitialRenderResult,
	data: undefined,
	isLoading: true,
	isFetching: true,
	error: null,
	refetch: vi.fn(),
} satisfies SpendReportQuery;

const renderView = (organization: typeof MockOrganization | undefined) => {
	const onOrganizationChange = vi.fn();
	render(
		<SpendPageView
			isEntitled
			isEnabled
			now={new Date("2026-03-12T12:00:00Z")}
			organizations={[MockOrganization, MockOrganization2]}
			organization={organization}
			onOrganizationChange={onOrganizationChange}
			isOrganizationsLoading={false}
			organizationsError={null}
			period={{
				start: new Date("2026-03-05T12:00:00Z"),
				end: new Date("2026-03-12T12:00:00Z"),
				preset: "last_7d",
			}}
			minDate={undefined}
			onPeriodChange={vi.fn()}
			filterMenus={undefined}
			reportQuery={pendingReportQuery}
		/>,
	);
	return { onOrganizationChange };
};

it("reports the organization picked from the switcher", async () => {
	const user = userEvent.setup();
	const { onOrganizationChange } = renderView(MockOrganization);

	await user.click(
		screen.getByRole("button", {
			name: `Organization ${MockOrganization.display_name}`,
		}),
	);
	await user.click(
		await screen.findByRole("option", { name: /My Organization 2/ }),
	);

	expect(onOrganizationChange).toHaveBeenCalledWith(MockOrganization2);
});

it("reports the organization picked to recover from a denied one", async () => {
	const user = userEvent.setup();
	const { onOrganizationChange } = renderView(undefined);

	await user.click(
		screen.getByRole("button", { name: /Select an organization/ }),
	);
	await user.click(
		await screen.findByRole("option", { name: /My Organization 2/ }),
	);

	expect(onOrganizationChange).toHaveBeenCalledWith(MockOrganization2);
});
