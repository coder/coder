import { fireEvent, renderHook, screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "#/testHelpers/server";
import { AttachmentBlock } from "./AttachmentBlocks";
import { FileProbeProvider, useFileProbes } from "./FileProbeContext";

describe("AttachmentBlock", () => {
	it("marks a deleted image expired even after the image unmounts", async () => {
		server.use(
			http.get(
				"/api/v2/chats/files/deleted-file",
				() => new HttpResponse(null, { status: 404 }),
			),
		);
		let showsImage = true;
		const { result, rerender } = renderHook(() => useFileProbes(), {
			wrapper: ({ children }) => (
				<FileProbeProvider evictedFileIds={new Set()}>
					{showsImage && (
						<AttachmentBlock
							block={{
								type: "file",
								media_type: "image/png",
								name: "deleted.png",
								file_id: "deleted-file",
							}}
							onImageClick={vi.fn()}
							onTextFileClick={vi.fn()}
						/>
					)}
					{children}
				</FileProbeProvider>
			),
		});

		fireEvent.error(screen.getByRole("img", { name: "deleted.png" }));
		showsImage = false;
		rerender();

		await waitFor(() =>
			expect(result.current.hasExpired("deleted-file")).toBe(true),
		);
	});
});
