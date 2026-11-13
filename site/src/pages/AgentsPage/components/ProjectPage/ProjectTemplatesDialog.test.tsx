import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { agentTemplates } from "#/api/queries/templates";
import {
	MockChatProject,
	MockTemplate,
	mockApiError,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { ProjectTemplatesDialog } from "./ProjectTemplatesDialog";

describe("ProjectTemplatesDialog", () => {
	it("clears the save error when the draft changes", async () => {
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(
			agentTemplates(MockChatProject.organization_id).queryKey,
			[MockTemplate],
		);
		const onResetError = vi.fn();
		render(
			<QueryClientProvider client={queryClient}>
				<ProjectTemplatesDialog
					project={MockChatProject}
					open
					onOpenChange={vi.fn()}
					isSaving={false}
					error={mockApiError({ message: "Failed to update chat project." })}
					onSave={vi.fn()}
					onResetError={onResetError}
				/>
			</QueryClientProvider>,
		);

		await userEvent.click(screen.getByRole("button", { name: "Select all" }));

		expect(onResetError).toHaveBeenCalled();
	});
});
