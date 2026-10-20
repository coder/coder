import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChatProject } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockDefaultOrganization,
	MockOrganization2,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { projectsSectionCollapsedStorageKey } from "../../../utils/projectsSectionStorage";
import { ProjectFolders } from "./ProjectFolders";

type ProjectFoldersOptions = {
	readonly error?: unknown;
	readonly currentUserId?: string;
};

const projectFoldersElement = (
	projects: readonly ChatProject[],
	{ error, currentUserId = MockUserOwner.id }: ProjectFoldersOptions = {},
) => (
	<MemoryRouter>
		<ProjectFolders
			currentUserId={currentUserId}
			projects={projects}
			organizations={[MockDefaultOrganization]}
			chatsByProjectId={new Map()}
			expandedProjectIds={{}}
			onToggle={vi.fn()}
			onOpenProjectDialog={vi.fn()}
			onDelete={vi.fn()}
			emptyMessage="No agents yet"
			isLoading={false}
			error={error}
			isLoadError={false}
			onRetry={vi.fn()}
		/>
	</MemoryRouter>
);

const renderProjectFolders = (
	projects: readonly ChatProject[],
	options?: ProjectFoldersOptions,
) => render(projectFoldersElement(projects, options));

const collapsedKey = projectsSectionCollapsedStorageKey(MockUserOwner.id);

describe("ProjectFolders", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("keeps cached folders usable when a refetch fails", async () => {
		const user = userEvent.setup();
		const onToggle = vi.fn();
		const onRetry = vi.fn();

		render(
			<MemoryRouter>
				<ProjectFolders
					currentUserId={MockUserOwner.id}
					projects={[MockChatProject]}
					organizations={[MockDefaultOrganization]}
					chatsByProjectId={new Map()}
					expandedProjectIds={{}}
					onToggle={onToggle}
					onOpenProjectDialog={vi.fn()}
					onDelete={vi.fn()}
					emptyMessage="No agents yet"
					isLoading={false}
					error={new Error("Projects unavailable")}
					isLoadError={false}
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

	it("keeps focus on the empty state's button when a refetch fails", async () => {
		const user = userEvent.setup();
		const { rerender } = renderProjectFolders([]);

		await user.tab();
		await user.tab();
		const newProjectButton = screen.getByRole("button", {
			name: "New project",
		});
		expect(newProjectButton).toHaveFocus();

		rerender(
			projectFoldersElement([], { error: new Error("Projects unavailable") }),
		);
		expect(screen.getByRole("button", { name: "New project" })).toHaveFocus();
	});

	it("collapses and expands the whole Projects section", async () => {
		const user = userEvent.setup();
		renderProjectFolders([MockChatProject]);

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

	it("remembers the Projects section collapse state across mounts", async () => {
		const user = userEvent.setup();
		const firstMount = renderProjectFolders([]);

		await user.click(screen.getByRole("button", { name: "Projects" }));
		expect(localStorage.getItem(collapsedKey)).toBe("true");
		firstMount.unmount();

		renderProjectFolders([]);
		await user.click(screen.getByRole("button", { name: "Projects" }));
		expect(localStorage.getItem(collapsedKey)).toBeNull();
	});

	it("keeps the saved collapse state separate per user", async () => {
		const user = userEvent.setup();
		const firstMount = renderProjectFolders([]);
		await user.click(screen.getByRole("button", { name: "Projects" }));
		firstMount.unmount();

		renderProjectFolders([], { currentUserId: MockUserMember.id });
		await user.click(screen.getByRole("button", { name: "Projects" }));
		expect(
			localStorage.getItem(
				projectsSectionCollapsedStorageKey(MockUserMember.id),
			),
		).toBe("true");
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
					currentUserId={MockUserOwner.id}
					projects={[MockChatProject, otherProject]}
					organizations={[MockDefaultOrganization, MockOrganization2]}
					chatsByProjectId={new Map()}
					expandedProjectIds={{}}
					onToggle={onToggle}
					onOpenProjectDialog={vi.fn()}
					onDelete={vi.fn()}
					emptyMessage="No agents yet"
					isLoading={false}
					error={undefined}
					isLoadError={false}
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
