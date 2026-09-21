import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { MockChatProject } from "#/testHelpers/entities";
import { ProjectFolders } from "./ProjectFolders";

describe("ProjectFolders", () => {
	it("keeps cached folders usable when a refetch fails", async () => {
		const user = userEvent.setup();
		const onToggle = vi.fn();
		const onRetry = vi.fn();

		render(
			<MemoryRouter>
				<ProjectFolders
					projects={[MockChatProject]}
					chatsByProjectId={new Map()}
					expandedProjectIds={{}}
					onToggle={onToggle}
					onCreate={vi.fn()}
					onEdit={vi.fn()}
					onDelete={vi.fn()}
					onRetryPermissions={vi.fn()}
					error={new Error("Projects unavailable")}
					onRetry={onRetry}
				/>
			</MemoryRouter>,
		);

		await user.click(
			screen.getByRole("button", { name: `Expand ${MockChatProject.name}` }),
		);
		expect(onToggle).toHaveBeenCalledWith(MockChatProject.id);

		await user.click(screen.getByRole("button", { name: "Retry" }));
		expect(onRetry).toHaveBeenCalledTimes(1);
	});
});
