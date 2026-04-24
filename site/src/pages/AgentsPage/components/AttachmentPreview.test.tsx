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

	it("does not render draft warnings when none are present", () => {
		const file = new File(["hello"], "note.txt", { type: "text/plain" });
		const uploadStates = new Map<File, UploadState>([
			[file, { status: "uploaded", fileId: "file-1" }],
		]);

		render(
			<AttachmentPreview
				attachments={[file]}
				onRemove={vi.fn()}
				uploadStates={uploadStates}
			/>,
		);

		expect(screen.queryByText(/save as a draft/i)).not.toBeInTheDocument();
	});

	it("shows draft warnings for mixed attachment states", () => {
		const uploading = new File(["hello"], "uploading.txt", {
			type: "text/plain",
		});
		const uploaded = new File(["world"], "uploaded.txt", {
			type: "text/plain",
		});
		const pendingWarning = "This file is attached for now, but it may be lost.";
		const uploadedWarning =
			"This file is usable in this session, but it could not be saved.";
		const uploadStates = new Map<File, UploadState>([
			[uploading, { status: "uploading", draftWarning: pendingWarning }],
			[
				uploaded,
				{
					status: "uploaded",
					fileId: "file-2",
					draftWarning: uploadedWarning,
				},
			],
		]);

		render(
			<AttachmentPreview
				attachments={[uploading, uploaded]}
				onRemove={vi.fn()}
				uploadStates={uploadStates}
			/>,
		);

		expect(screen.getByText(pendingWarning)).toBeInTheDocument();
		expect(screen.getByText(uploadedWarning)).toBeInTheDocument();
	});

	it("renders nothing without attachments", () => {
		const { container } = render(
			<AttachmentPreview attachments={[]} onRemove={vi.fn()} />,
		);

		expect(container.firstChild).toBeNull();
	});
});
