import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useRef } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, useLocation } from "react-router";
import { describe, expect, it, onTestFinished, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProject } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockChatProjectSummary,
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { hasHighlightComposerState } from "../../../utils/composerHighlight";
import { buildAgentProjectPath } from "../../../utils/navigation";
import { ProjectFolders } from "./ProjectFolders";

type ProjectFoldersOptions = {
	readonly error?: unknown;
	readonly route?: string;
	readonly withComposer?: boolean;
	readonly onToggle?: (projectId: string) => void;
	readonly onRetry?: () => void;
};

// Stands in for the project page's composer, which takes focus when a New
// chat navigation asks for the highlight.
const ComposerStandIn: React.FC = () => {
	const location = useLocation();
	const inputRef = useRef<HTMLInputElement>(null);
	const highlightKey = hasHighlightComposerState(location.state)
		? location.key
		: undefined;
	useEffect(() => {
		if (highlightKey !== undefined) {
			inputRef.current?.focus();
		}
	}, [highlightKey]);
	return <input ref={inputRef} aria-label="Composer" />;
};

const renderProjectFolders = (
	projects: readonly ChatProject[],
	{
		error,
		route = "/agents",
		withComposer = false,
		onToggle = vi.fn(),
		onRetry = vi.fn(),
	}: ProjectFoldersOptions = {},
) =>
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<MemoryRouter initialEntries={[route]}>
				<ProjectFolders
					pinnedProjects={projects.map((project) => ({
						...MockChatProjectSummary,
						project,
					}))}
					dragInstructionsId="drag-instructions"
					organizations={[MockDefaultOrganization, MockOrganization2]}
					chatsByProjectId={new Map()}
					pinnedChatsByProjectId={new Map()}
					chatListInput={{}}
					expandedProjectIds={{}}
					onToggle={onToggle}
					onEdit={vi.fn()}
					onDelete={vi.fn()}
					emptyMessage="No chats yet"
					isLoading={false}
					error={error}
					onRetry={onRetry}
				/>
				{withComposer && <ComposerStandIn />}
			</MemoryRouter>
		</QueryClientProvider>,
	);

describe("ProjectFolders", () => {
	it("keeps cached folders usable when a refetch fails", async () => {
		const user = userEvent.setup();
		const onToggle = vi.fn();
		const onRetry = vi.fn();
		renderProjectFolders([MockChatProject], {
			error: new Error("Projects unavailable"),
			onToggle,
			onRetry,
		});

		await user.click(
			screen.getByRole("button", { name: `Expand ${MockChatProject.name}` }),
		);
		expect(onToggle).toHaveBeenCalledWith(MockChatProject.id);

		await user.click(screen.getByRole("button", { name: "Retry" }));
		expect(onRetry).toHaveBeenCalledTimes(1);
	});

	it("leaves focus in the composer after New chat on the project's page", async () => {
		// In browsers the navigation commits, and the composer takes focus,
		// before the closing menu's timer returns focus to its button. Fake
		// timers hold that timer so the test can reproduce the order.
		vi.useFakeTimers();
		onTestFinished(() => {
			vi.useRealTimers();
		});
		const user = userEvent.setup({ delay: null });
		renderProjectFolders([MockChatProject], {
			route: buildAgentProjectPath({ projectId: MockChatProject.id }),
			withComposer: true,
		});

		await user.click(
			screen.getByRole("button", {
				name: `Open project actions for ${MockChatProject.name}`,
			}),
		);
		await user.click(screen.getByRole("menuitem", { name: "New chat" }));
		await act(async () => {});
		const composer = screen.getByRole("textbox", { name: "Composer" });
		expect(composer).toHaveFocus();
		act(() => {
			vi.runOnlyPendingTimers();
		});

		expect(composer).toHaveFocus();
	});

	it("unpins a project from its actions menu, reached by keyboard", async () => {
		const user = userEvent.setup();
		const unpin = vi
			.spyOn(API.experimental, "unpinChatProject")
			.mockResolvedValue();
		onTestFinished(() => unpin.mockRestore());
		renderProjectFolders([MockChatProject]);

		act(() => {
			screen.getByRole("link", { name: MockChatProject.name }).focus();
		});
		await user.tab();
		expect(
			screen.getByRole("button", {
				name: `Open project actions for ${MockChatProject.name}`,
			}),
		).toHaveFocus();
		await user.keyboard("{Enter}");
		await user.click(screen.getByRole("menuitem", { name: "Unpin" }));
		expect(unpin).toHaveBeenCalledWith(
			MockChatProject.organization_id,
			MockChatProject.id,
		);
	});

	it("names same-named projects by their organization", async () => {
		const user = userEvent.setup();
		const onToggle = vi.fn();
		const otherProject = {
			...MockChatProject,
			id: "chat-project-2",
			organization_id: MockOrganization2.id,
		};

		renderProjectFolders([MockChatProject, otherProject], { onToggle });

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
