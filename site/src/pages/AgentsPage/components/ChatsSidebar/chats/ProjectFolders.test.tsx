import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import {
	MockChatProject,
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
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
					organizations={[MockDefaultOrganization]}
					chatsByProjectId={new Map()}
					projectIdsWithPinnedChats={new Set()}
					expandedProjectIds={{}}
					onToggle={onToggle}
					onOpenProjectDialog={vi.fn()}
					onDelete={vi.fn()}
					emptyMessage="No agents yet"
					isLoading={false}
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

	it("collapses and expands the whole Projects section", async () => {
		const user = userEvent.setup();

		render(
			<MemoryRouter>
				<ProjectFolders
					projects={[MockChatProject]}
					organizations={[MockDefaultOrganization]}
					chatsByProjectId={new Map()}
					projectIdsWithPinnedChats={new Set()}
					expandedProjectIds={{}}
					onToggle={vi.fn()}
					onOpenProjectDialog={vi.fn()}
					onDelete={vi.fn()}
					emptyMessage="No agents yet"
					isLoading={false}
					error={undefined}
					onRetry={vi.fn()}
				/>
			</MemoryRouter>,
		);

		const sectionToggle = screen.getByRole("button", { name: "Projects" });
		await user.click(sectionToggle);
		expect(sectionToggle).toHaveAttribute("aria-expanded", "false");
		expect(
			screen.queryByRole("link", { name: MockChatProject.name }),
		).not.toBeInTheDocument();
		expect(screen.getByRole("button", { name: "New project" })).toBeVisible();

		await user.click(sectionToggle);
		expect(sectionToggle).toHaveAttribute("aria-expanded", "true");
		expect(
			screen.getByRole("link", { name: MockChatProject.name }),
		).toBeVisible();
	});

	it("names same-named projects by their organization", async () => {
		const user = userEvent.setup();
		const onToggle = vi.fn();
		const otherProject = {
			...MockChatProject,
			id: "chat-project-2",
			organization_id: MockOrganization2.id,
		};

		render(
			<MemoryRouter>
				<ProjectFolders
					projects={[MockChatProject, otherProject]}
					organizations={[MockDefaultOrganization, MockOrganization2]}
					chatsByProjectId={new Map()}
					projectIdsWithPinnedChats={new Set()}
					expandedProjectIds={{}}
					onToggle={onToggle}
					onOpenProjectDialog={vi.fn()}
					onDelete={vi.fn()}
					emptyMessage="No agents yet"
					isLoading={false}
					error={undefined}
					onRetry={vi.fn()}
				/>
			</MemoryRouter>,
		);

		const otherLabel = `${otherProject.name} (${MockOrganization2.display_name})`;
		const defaultLabel = `${MockChatProject.name} (${MockDefaultOrganization.display_name})`;
		expect(screen.getByRole("link", { name: otherLabel })).toBeVisible();
		expect(screen.getByRole("link", { name: defaultLabel })).toBeVisible();
		expect(
			screen.getByRole("button", {
				name: `Open project actions for ${otherLabel}`,
			}),
		).toBeInTheDocument();

		await user.click(
			screen.getByRole("button", { name: `Expand ${otherLabel}` }),
		);
		expect(onToggle).toHaveBeenCalledWith(otherProject.id);
	});
});
