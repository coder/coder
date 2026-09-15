import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MockChatProject } from "#/testHelpers/entities";
import { ChatProjectDialog } from "./ChatProjectDialog";

describe("ChatProjectDialog", () => {
	it("submits a create request", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(async () => {});
		render(
			<ChatProjectDialog
				organizationId="organization-1"
				open
				onOpenChange={vi.fn()}
				onSubmit={onSubmit}
			/>,
		);

		await user.type(screen.getByLabelText("Name"), "Launch");
		await user.type(screen.getByLabelText("Description"), "Release work");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).toHaveBeenCalledWith({
			organization_id: "organization-1",
			name: "Launch",
			description: "Release work",
		});
	});

	it("submits the selected project's name and description", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn(async () => {});
		const { rerender } = render(
			<ChatProjectDialog
				key="closed"
				organizationId={MockChatProject.organization_id}
				open={false}
				onOpenChange={vi.fn()}
				onSubmit={onSubmit}
			/>,
		);

		rerender(
			<ChatProjectDialog
				key={MockChatProject.id}
				organizationId={MockChatProject.organization_id}
				project={MockChatProject}
				open
				onOpenChange={vi.fn()}
				onSubmit={onSubmit}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onSubmit).toHaveBeenCalledWith({
			name: MockChatProject.name,
			description: MockChatProject.description,
		});
	});
});
