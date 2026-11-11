import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import type { WorkspaceACL } from "#/api/typesGenerated";
import {
	MockGroup,
	MockUserMember,
	MockWorkspace,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { RoleSelectField, WorkspaceSharingForm } from "./WorkspaceSharingForm";

const mockWorkspaceACL: WorkspaceACL = {
	users: [
		{
			id: MockUserMember.id,
			username: MockUserMember.username,
			name: MockUserMember.name,
			avatar_url: MockUserMember.avatar_url,
			role: "use",
		},
	],
	group: [{ ...MockGroup, role: "use" }],
};

describe("WorkspaceSharingForm", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("names each member's role select after the member", async () => {
		vi.spyOn(API, "getWorkspaceSharingSettings").mockResolvedValue({
			sharing_disabled: false,
			sharing_globally_disabled: false,
			shareable_workspace_owners: "everyone",
		});
		const onUpdateUser = vi.fn();
		const onUpdateGroup = vi.fn();

		render(
			<WorkspaceSharingForm
				organizationId={MockWorkspace.organization_id}
				workspaceACL={mockWorkspaceACL}
				canShareWorkspace
				error={undefined}
				onUpdateUser={onUpdateUser}
				updatingUserId={undefined}
				onRemoveUser={vi.fn()}
				onUpdateGroup={onUpdateGroup}
				onRemoveGroup={vi.fn()}
			/>,
		);

		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("combobox", {
				name: `Role for ${MockUserMember.username}`,
			}),
		);
		await user.click(await screen.findByRole("option", { name: /Admin/ }));
		expect(onUpdateUser).toHaveBeenCalledWith(
			mockWorkspaceACL.users[0],
			"admin",
		);

		await user.click(
			screen.getByRole("combobox", {
				name: `Role for ${MockGroup.display_name || MockGroup.name}`,
			}),
		);
		await user.click(await screen.findByRole("option", { name: /Admin/ }));
		expect(onUpdateGroup).toHaveBeenCalledWith(
			mockWorkspaceACL.group[0],
			"admin",
		);
	});

	it("names each member's menu after the member", async () => {
		vi.spyOn(API, "getWorkspaceSharingSettings").mockResolvedValue({
			sharing_disabled: false,
			sharing_globally_disabled: false,
			shareable_workspace_owners: "everyone",
		});
		const onRemoveUser = vi.fn();
		const onRemoveGroup = vi.fn();

		render(
			<WorkspaceSharingForm
				organizationId={MockWorkspace.organization_id}
				workspaceACL={mockWorkspaceACL}
				canShareWorkspace
				error={undefined}
				onUpdateUser={vi.fn()}
				updatingUserId={undefined}
				onRemoveUser={onRemoveUser}
				onUpdateGroup={vi.fn()}
				onRemoveGroup={onRemoveGroup}
			/>,
		);

		const user = userEvent.setup();
		await user.click(
			await screen.findByRole("button", {
				name: `Open menu for ${MockUserMember.username}`,
			}),
		);
		await user.click(await screen.findByRole("menuitem", { name: "Remove" }));
		expect(onRemoveUser).toHaveBeenCalledWith(mockWorkspaceACL.users[0]);

		await user.click(
			screen.getByRole("button", {
				name: `Open menu for ${MockGroup.display_name || MockGroup.name}`,
			}),
		);
		await user.click(await screen.findByRole("menuitem", { name: "Remove" }));
		expect(onRemoveGroup).toHaveBeenCalledWith(mockWorkspaceACL.group[0]);
	});

	it("renders headers and members in one table when compact", async () => {
		vi.spyOn(API, "getWorkspaceSharingSettings").mockResolvedValue({
			sharing_disabled: false,
			sharing_globally_disabled: false,
			shareable_workspace_owners: "everyone",
		});

		render(
			<WorkspaceSharingForm
				organizationId={MockWorkspace.organization_id}
				workspaceACL={mockWorkspaceACL}
				canShareWorkspace
				error={undefined}
				onUpdateUser={vi.fn()}
				updatingUserId={undefined}
				onRemoveUser={vi.fn()}
				onUpdateGroup={vi.fn()}
				onRemoveGroup={vi.fn()}
				isCompact
			/>,
		);

		await screen.findByRole("columnheader", { name: "Member" });
		const tables = screen.getAllByRole("table");
		expect(tables).toHaveLength(1);
		expect(within(tables[0]).getAllByRole("row")).toHaveLength(3);
	});

	it("lets keyboard users reach the compact list when it has no controls", async () => {
		vi.spyOn(API, "getWorkspaceSharingSettings").mockResolvedValue({
			sharing_disabled: false,
			sharing_globally_disabled: false,
			shareable_workspace_owners: "everyone",
		});

		render(
			<WorkspaceSharingForm
				organizationId={MockWorkspace.organization_id}
				workspaceACL={mockWorkspaceACL}
				canShareWorkspace={false}
				error={undefined}
				onUpdateUser={vi.fn()}
				updatingUserId={undefined}
				onRemoveUser={vi.fn()}
				onUpdateGroup={vi.fn()}
				onRemoveGroup={vi.fn()}
				isCompact
			/>,
		);

		const list = await screen.findByRole("region", {
			name: "Shared workspace members and groups",
		});
		await userEvent.setup().tab();
		expect(list).toHaveFocus();
	});
});

describe("RoleSelectField", () => {
	it("has an accessible name", async () => {
		const onChange = vi.fn();
		render(<RoleSelectField value="use" onChange={onChange} />);

		const user = userEvent.setup();
		await user.click(await screen.findByRole("combobox", { name: "Role" }));
		await user.click(await screen.findByRole("option", { name: "Admin" }));
		expect(onChange).toHaveBeenCalledWith("admin");
	});
});
