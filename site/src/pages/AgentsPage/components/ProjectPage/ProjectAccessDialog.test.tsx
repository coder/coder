import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type {
	ChatProjectACL,
	Group,
	OrganizationMemberWithUserData,
} from "#/api/typesGenerated";
import {
	MockChatProject,
	MockGroup,
	MockOrganizationMember,
	MockOrganizationMember2,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { ProjectAccessDialog } from "./ProjectAccessDialog";
import { buildAccessEntries } from "./projectAccess";

const MockOtherMember: OrganizationMemberWithUserData = {
	...MockOrganizationMember2,
	user_id: "test-user-3",
	username: "TestUser3",
	email: "test3@coder.com",
	name: "",
};

const MockOtherGroup: Group = {
	...MockGroup,
	id: "other-group",
	name: "Back-End",
	display_name: "Back-End",
};

const mockACL: ChatProjectACL = {
	users: [{ ...MockUserMember, role: "admin" }],
	groups: [],
};

type DialogProps = React.ComponentProps<typeof ProjectAccessDialog>;

const renderDialog = (props: Partial<DialogProps> = {}) => {
	const callbacks = {
		onAdd: vi.fn<DialogProps["onAdd"]>(),
		onChangeRole: vi.fn<DialogProps["onChangeRole"]>(),
		onRemove: vi.fn<DialogProps["onRemove"]>(),
	};
	renderComponent(
		<QueryClientProvider client={createTestQueryClient()}>
			<ProjectAccessDialog
				open
				onOpenChange={vi.fn()}
				entries={buildAccessEntries(MockUserOwner.id, MockUserOwner, mockACL)}
				error={undefined}
				isRetrying={false}
				onRetry={vi.fn()}
				onCloseAutoFocus={vi.fn()}
				canManage
				organizationId={MockChatProject.organization_id}
				currentUserId={MockUserOwner.id}
				pendingPrincipalId={undefined}
				mutationError={undefined}
				{...callbacks}
				{...props}
			/>
		</QueryClientProvider>,
	);
	return { user: userEvent.setup(), ...callbacks };
};

const getAutocomplete = () =>
	screen.getByRole("button", { name: "User or group" });

const openAutocomplete = (user: ReturnType<typeof userEvent.setup>) =>
	user.click(getAutocomplete());

const chooseAddRole = async (
	user: ReturnType<typeof userEvent.setup>,
	role: "Admin" | "Member",
) => {
	await user.click(screen.getByRole("combobox", { name: "Role" }));
	await user.click(
		await screen.findByRole("option", { name: new RegExp(role) }),
	);
};

describe("ProjectAccessDialog", () => {
	beforeEach(() => {
		vi.spyOn(API, "getOrganizationPaginatedMembers").mockResolvedValue({
			members: [
				MockOrganizationMember,
				MockOrganizationMember2,
				MockOtherMember,
			],
			count: 3,
		});
		vi.spyOn(API, "getGroupsByOrganization").mockResolvedValue([
			MockGroup,
			MockOtherGroup,
		]);
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("adds the selected user as an admin, then clears the form", async () => {
		const { user, onAdd } = renderDialog();

		await openAutocomplete(user);
		await user.click(await screen.findByRole("option", { name: /TestUser3/ }));
		await chooseAddRole(user, "Admin");
		await user.click(screen.getByRole("button", { name: "Add" }));

		expect(onAdd).toHaveBeenCalledWith(
			{ kind: "user", id: MockOtherMember.user_id },
			"admin",
			expect.any(Function),
		);
		act(() => onAdd.mock.calls[0][2]());
		expect(getAutocomplete()).toHaveFocus();
		await user.click(screen.getByRole("button", { name: "Add" }));
		expect(onAdd).toHaveBeenCalledTimes(1);
	});

	it("adds the selected group as a member", async () => {
		const { user, onAdd } = renderDialog();

		await openAutocomplete(user);
		await user.click(await screen.findByRole("option", { name: /Back-End/ }));
		await chooseAddRole(user, "Member");
		await user.click(screen.getByRole("button", { name: "Add" }));

		expect(onAdd).toHaveBeenCalledWith(
			{ kind: "group", id: MockOtherGroup.id },
			"use",
			expect.any(Function),
		);
	});

	it("does not add a user or group without a role", async () => {
		const { user, onAdd } = renderDialog();

		await openAutocomplete(user);
		await user.click(await screen.findByRole("option", { name: /TestUser3/ }));
		await user.click(screen.getByRole("button", { name: "Add" }));

		expect(onAdd).not.toHaveBeenCalled();
	});

	it("offers neither the owner, the viewer, nor anyone already listed", async () => {
		// Site admins can manage access without an entry of their own.
		const { user } = renderDialog({
			currentUserId: MockUserMember.id,
			entries: buildAccessEntries(MockUserOwner.id, MockUserOwner, {
				users: [],
				groups: [{ ...MockGroup, role: "use" }],
			}),
		});

		await openAutocomplete(user);
		await screen.findByRole("option", { name: /TestUser3/ });

		expect(
			screen.getAllByRole("option").map((option) => option.textContent),
		).toEqual([
			expect.stringContaining("Back-End"),
			expect.stringContaining("TestUser3"),
		]);
	});

	it("changes a listed user's role", async () => {
		const { user, onChangeRole } = renderDialog();

		await user.click(
			screen.getByRole("combobox", { name: `Role for ${MockUserMember.name}` }),
		);
		await user.click(await screen.findByRole("option", { name: /Member/ }));

		await waitFor(() =>
			expect(onChangeRole).toHaveBeenCalledWith(
				expect.objectContaining({ kind: "user", id: MockUserMember.id }),
				"use",
			),
		);
	});

	it("removes a listed user", async () => {
		const { user, onRemove } = renderDialog();

		await user.click(
			screen.getByRole("button", {
				name: `Open menu for ${MockUserMember.name}`,
			}),
		);
		await user.click(await screen.findByRole("menuitem", { name: "Remove" }));

		expect(onRemove).toHaveBeenCalledWith(
			expect.objectContaining({ kind: "user", id: MockUserMember.id }),
			expect.any(Function),
		);
		act(() => onRemove.mock.calls[0][1]());
		expect(getAutocomplete()).toHaveFocus();
	});

	it("ignores adds and row changes while another change saves", async () => {
		const { user, onAdd, onChangeRole, onRemove } = renderDialog({
			pendingPrincipalId: MockOtherGroup.id,
		});

		await openAutocomplete(user);
		await user.click(await screen.findByRole("option", { name: /TestUser3/ }));
		await chooseAddRole(user, "Admin");
		await user.click(screen.getByRole("button", { name: "Add" }));
		// Typing "m" picks Member, and Enter opens the menu, then removes.
		const name = MockUserMember.name;
		act(() =>
			screen.getByRole("combobox", { name: `Role for ${name}` }).focus(),
		);
		await user.keyboard("m");
		act(() =>
			screen.getByRole("button", { name: `Open menu for ${name}` }).focus(),
		);
		await user.keyboard("{Enter}{Enter}");

		expect(onAdd).not.toHaveBeenCalled();
		expect(onChangeRole).not.toHaveBeenCalled();
		expect(onRemove).not.toHaveBeenCalled();
	});
});
