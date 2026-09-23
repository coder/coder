import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MockChatProject } from "#/testHelpers/entities";
import { ChatProjectDialog } from "./ChatProjectDialog";

describe("ChatProjectDialog", () => {
	it("submits the selected project's name and description", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const { rerender } = render(
			<ChatProjectDialog
				open={false}
				onOpenChange={vi.fn()}
				isSubmitting={false}
				error={undefined}
				onSubmit={onSubmit}
			/>,
		);

		rerender(
			<ChatProjectDialog
				project={MockChatProject}
				open
				onOpenChange={vi.fn()}
				isSubmitting={false}
				error={undefined}
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
