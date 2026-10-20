import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMockFile } from "#/testHelpers/files";
import { AttachmentPreview, type UploadState } from "./AttachmentPreview";

afterEach(() => {
	vi.restoreAllMocks();
});

describe("AttachmentPreview", () => {
	it("never loads the content of a workspace text file", async () => {
		const user = userEvent.setup();
		const fetchSpy = vi
			.spyOn(globalThis, "fetch")
			.mockResolvedValue(new Response("log contents"));
		const onRemove = vi.fn();
		const onTextPreview = vi.fn();
		const onInlineText = vi.fn();
		const file = createMockFile("build.log", "text/plain");

		render(
			<AttachmentPreview
				attachments={[file]}
				onRemove={onRemove}
				uploadStates={
					new Map<File, UploadState>([
						[
							file,
							{ status: "uploaded", fileId: "file-log", purpose: "workspace" },
						],
					])
				}
				onPreview={vi.fn()}
				onTextPreview={onTextPreview}
				onInlineText={onInlineText}
			/>,
		);
		for (const button of screen.getAllByRole("button")) {
			await user.click(button);
		}

		expect(onRemove).toHaveBeenCalledWith(file);
		expect(fetchSpy).not.toHaveBeenCalled();
		expect(onTextPreview).not.toHaveBeenCalled();
		expect(onInlineText).not.toHaveBeenCalled();
	});
});
