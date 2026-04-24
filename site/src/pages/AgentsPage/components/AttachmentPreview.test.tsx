import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AttachmentPreview, type UploadState } from "./AttachmentPreview";

describe("AttachmentPreview", () => {
	it("shows draft preservation warnings near attachments", () => {
		const file = new File(["hello"], "note.txt", { type: "text/plain" });
		const warning =
			"This file is attached for now, but it is too large to save as a draft.";
		const uploadStates = new Map<File, UploadState>([
			[file, { status: "uploading", draftWarning: warning }],
		]);

		render(
			<AttachmentPreview
				attachments={[file]}
				onRemove={vi.fn()}
				uploadStates={uploadStates}
			/>,
		);

		expect(screen.getByText(warning)).toBeInTheDocument();
	});
});
