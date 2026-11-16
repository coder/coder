import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import {
	MockGroup,
	MockOrganizationMember,
	MockWorkspace,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { UserOrGroupAutocomplete } from "./UserOrGroupAutocomplete";

describe("UserOrGroupAutocomplete", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("keeps its label in the name after a selection", async () => {
		vi.spyOn(API, "getGroupsByOrganization").mockResolvedValue([MockGroup]);
		vi.spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
			members: [MockOrganizationMember],
			count: 1,
		});
		const onChange = vi.fn();
		render(
			<UserOrGroupAutocomplete
				value={MockGroup}
				onChange={onChange}
				organizationId={MockWorkspace.organization_id}
				exclude={[]}
			/>,
		);

		const user = userEvent.setup();
		await user.click(
			screen.getByRole("button", {
				name: `Search for user or group ${MockGroup.display_name}`,
			}),
		);
		await user.click(
			await screen.findByRole("option", {
				name: new RegExp(MockOrganizationMember.username),
			}),
		);
		expect(onChange).toHaveBeenCalledWith(
			expect.objectContaining({ user_id: MockOrganizationMember.user_id }),
		);
	});
});
