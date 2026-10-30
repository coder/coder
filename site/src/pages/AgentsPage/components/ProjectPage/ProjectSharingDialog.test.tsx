import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { chatProjectACLAvailable } from "#/api/queries/chatProjects";
import {
	MockChatProject,
	MockChatProjectACL,
	MockUserMember,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { ProjectSharingDialogView } from "./ProjectSharingDialog";

describe("ProjectSharingDialogView", () => {
	it("keeps a role picker focusable but ignores it while a change saves", async () => {
		const onChangeRole = vi.fn();
		render(
			<ProjectSharingDialogView
				project={MockChatProject}
				acl={MockChatProjectACL}
				loadError={undefined}
				saveError={undefined}
				isSaving
				availableQueryOptions={(options) =>
					chatProjectACLAvailable(MockChatProject, options)
				}
				onAdd={vi.fn()}
				onChangeRole={onChangeRole}
				onRemove={vi.fn()}
				onClose={vi.fn()}
			/>,
		);
		const user = userEvent.setup();
		const rolePicker = await screen.findByRole("combobox", {
			name: `Role for ${MockUserMember.name || MockUserMember.username}`,
		});

		await user.click(rolePicker);
		await user.keyboard("{ArrowDown}{Enter}");

		expect(rolePicker).toHaveFocus();
		expect(onChangeRole).not.toHaveBeenCalled();
	});
});
