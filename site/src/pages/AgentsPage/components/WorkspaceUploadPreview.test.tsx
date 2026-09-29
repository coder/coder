import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { createMockFile } from "#/testHelpers/files";
import { renderComponent } from "#/testHelpers/renderHelpers";
import type { WorkspaceFileUpload } from "../hooks/useWorkspaceFileUploads";
import { WorkspaceUploadPreview } from "./WorkspaceUploadPreview";

const uploadedEntry: WorkspaceFileUpload = {
	id: "uploaded-design-handoff.zip",
	file: createMockFile("design-handoff.zip", "application/zip"),
	status: "uploaded",
	response: {
		path: "/home/coder/.coder/chats/chat-1/files/design-handoff.zip",
		name: "design-handoff.zip",
		size: 4096,
		media_type: "application/zip",
		workspace_id: "ws-1",
	},
};

describe("WorkspaceUploadPreview", () => {
	it("reports the removed entry by id", async () => {
		const user = userEvent.setup();
		const onRemove = vi.fn();
		renderComponent(
			<WorkspaceUploadPreview uploads={[uploadedEntry]} onRemove={onRemove} />,
		);

		await user.click(
			screen.getByRole("button", { name: "Remove design-handoff.zip" }),
		);

		expect(onRemove).toHaveBeenCalledWith("uploaded-design-handoff.zip");
	});
});
