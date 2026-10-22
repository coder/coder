import { fireEvent, renderHook, screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "#/testHelpers/server";
import { FileProbeProvider, useFileProbes } from "./FileProbeContext";
import { useAttachmentFailure } from "./useAttachmentFailure";

const deletedFileHref = "/api/v2/chats/files/deleted-file";

const DeletedImage: React.FC = () => {
	const [, onError] = useAttachmentFailure("deleted-file", deletedFileHref);
	return <img alt="" src={deletedFileHref} onError={onError} />;
};

describe("useAttachmentFailure", () => {
	it("marks a deleted file expired even after its image unmounts", async () => {
		server.use(
			http.get(deletedFileHref, () => new HttpResponse(null, { status: 404 })),
		);
		let showsImage = true;
		const { result, rerender } = renderHook(() => useFileProbes(), {
			wrapper: ({ children }) => (
				<FileProbeProvider evictedFileIds={new Set()}>
					{showsImage && <DeletedImage />}
					{children}
				</FileProbeProvider>
			),
		});

		fireEvent.error(screen.getByRole("img"));
		showsImage = false;
		rerender();

		await waitFor(() =>
			expect(result.current.hasExpired("deleted-file")).toBe(true),
		);
	});
});
