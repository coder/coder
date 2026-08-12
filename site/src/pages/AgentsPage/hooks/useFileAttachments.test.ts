import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import {
	persistedAttachmentsStorageKey,
	useFileAttachments,
} from "./useFileAttachments";

const persistEntry = (fileId: string, fileName: string, orgId: string) => ({
	fileId,
	fileName,
	fileType: "text/plain",
	lastModified: 1000,
	organizationId: orgId,
});

const uploadedFileIds = (
	result: ReturnType<typeof useFileAttachments>,
): string[] =>
	[...result.uploadStates.values()].flatMap((state) =>
		state.status === "uploaded" && state.fileId ? [state.fileId] : [],
	);

describe("useFileAttachments org scoping", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("defers restoration until the org is known", () => {
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([persistEntry("file-b", "b.txt", "org-b")]),
		);
		const { result, rerender } = renderHook(
			({ orgId }: { orgId: string | undefined }) =>
				useFileAttachments(orgId, { persist: true }),
			{ initialProps: { orgId: undefined as string | undefined } },
		);

		expect(result.current.attachments).toHaveLength(0);
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toContain(
			"file-b",
		);

		rerender({ orgId: "org-b" });
		expect(uploadedFileIds(result.current)).toStrictEqual(["file-b"]);
	});

	it("drops another org's attachments when the org changes", async () => {
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([persistEntry("file-a", "a.txt", "org-a")]),
		);
		const { result, rerender } = renderHook(
			({ orgId }: { orgId: string | undefined }) =>
				useFileAttachments(orgId, { persist: true }),
			{ initialProps: { orgId: "org-a" as string | undefined } },
		);
		expect(uploadedFileIds(result.current)).toStrictEqual(["file-a"]);

		// A send after this switch must not carry org-a's file IDs.
		rerender({ orgId: "org-b" });
		await waitFor(() => {
			expect(uploadedFileIds(result.current)).toStrictEqual([]);
		});
		expect(
			localStorage.getItem(persistedAttachmentsStorageKey) ?? "",
		).not.toContain("file-a");
	});
});
