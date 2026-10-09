import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockEntitlements,
	MockOrganization,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { TemplatesFilter } from "./TemplatesFilter";

const createFilter = (): UseFilterResult => ({
	query: "",
	values: {},
	used: false,
	update: vi.fn(),
	debounceUpdate: vi.fn(),
	cancelDebounce: vi.fn(),
});

// An organization the user has template permissions in but is not a member of.
// Its templates show up in the table, so the picker has to offer it too.
const nonMemberOrganization = {
	...MockOrganization,
	id: "d1b4a2ee-9d2e-4c9b-9c0f-2b2f0f0c9a11",
	name: "not-a-member",
	display_name: "Not A Member",
};

describe("TemplatesFilter", () => {
	it("offers organizations the user is not a member of", async () => {
		vi.spyOn(API, "getOrganizations").mockResolvedValue([
			MockOrganization,
			nonMemberOrganization,
		]);
		vi.spyOn(API, "getMyOrganizations").mockResolvedValue([MockOrganization]);

		renderWithAuth(
			<DashboardContext.Provider
				value={{
					entitlements: MockEntitlements,
					experiments: [],
					appearance: MockAppearanceConfig,
					buildInfo: MockBuildInfo,
					organizations: [MockOrganization],
					showOrganizations: true,
					canViewOrganizationSettings: true,
				}}
			>
				<TemplatesFilter filter={createFilter()} error={undefined} />
			</DashboardContext.Provider>,
		);

		await userEvent.click(
			await screen.findByLabelText("Select an organization"),
		);

		await waitFor(() => {
			expect(
				screen.getByText(nonMemberOrganization.display_name),
			).toBeInTheDocument();
		});
	});
});
