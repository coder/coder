import { act, render, renderHook, waitFor } from "@testing-library/react";
import { type FC, Suspense } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { createDeferred } from "#/testHelpers/deferred";
import { persistedAttachmentsStorage } from "#/utils/storage/keys";
import { useFileAttachments } from "./useFileAttachments";

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

const renderAttachments = (initialProps: { orgId: string | undefined }) =>
	renderHook(
		({ orgId }: { orgId: string | undefined }) =>
			useFileAttachments(orgId, { persist: true }),
		{ initialProps },
	);

const mockDeferredUpload = (): ((value: { id: string }) => void) => {
	const deferred = createDeferred<{ id: string }>();
	vi.spyOn(API.experimental, "uploadChatFile").mockReturnValue(
		deferred.promise,
	);
	return deferred.resolve;
};

describe("useFileAttachments org scoping", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("defers restoration until the org is known", () => {
		localStorage.setItem(
			persistedAttachmentsStorage.key,
			JSON.stringify([persistEntry("file-b", "b.txt", "org-b")]),
		);
		const { result, rerender } = renderAttachments({ orgId: undefined });

		expect(result.current.attachments).toHaveLength(0);
		expect(localStorage.getItem(persistedAttachmentsStorage.key)).toContain(
			"file-b",
		);

		rerender({ orgId: "org-b" });
		expect(uploadedFileIds(result.current)).toStrictEqual(["file-b"]);
	});

	it("drops another org's attachments when the org changes", async () => {
		localStorage.setItem(
			persistedAttachmentsStorage.key,
			JSON.stringify([persistEntry("file-a", "a.txt", "org-a")]),
		);
		const { result, rerender } = renderAttachments({ orgId: "org-a" });
		expect(uploadedFileIds(result.current)).toStrictEqual(["file-a"]);
		rerender({ orgId: "org-b" });
		await waitFor(() => {
			expect(uploadedFileIds(result.current)).toStrictEqual([]);
		});
		expect(
			localStorage.getItem(persistedAttachmentsStorage.key) ?? "",
		).not.toContain("file-a");
	});

	it("never exposes the previous org's file IDs in any render", async () => {
		localStorage.setItem(
			persistedAttachmentsStorage.key,
			JSON.stringify([persistEntry("file-a", "a.txt", "org-a")]),
		);
		// Log the hook output of every render, including the
		// intermediate commit between the org changing and the
		// adoption effect running; that window must expose nothing.
		const renderLog: { orgId: string; fileIds: string[] }[] = [];
		const Probe: FC<{ orgId: string }> = ({ orgId }) => {
			const result = useFileAttachments(orgId, { persist: true });
			renderLog.push({ orgId, fileIds: uploadedFileIds(result) });
			return null;
		};
		const { rerender } = render(<Probe orgId="org-a" />);
		expect(renderLog.at(-1)).toStrictEqual({
			orgId: "org-a",
			fileIds: ["file-a"],
		});

		rerender(<Probe orgId="org-b" />);
		await waitFor(() => {
			expect(renderLog.at(-1)?.fileIds).toStrictEqual([]);
		});
		const leaked = renderLog.filter(
			(entry) => entry.orgId === "org-b" && entry.fileIds.includes("file-a"),
		);
		expect(leaked).toStrictEqual([]);
	});

	it("reports adoption only after the post-commit effect", () => {
		localStorage.setItem(
			persistedAttachmentsStorage.key,
			JSON.stringify([persistEntry("file-a", "a.txt", "org-a")]),
		);
		const log: { adopted: boolean; fileIds: string[] }[] = [];
		const Probe: FC<{ orgId: string }> = ({ orgId }) => {
			const result = useFileAttachments(orgId, { persist: true });
			log.push({
				adopted: result.organizationAdopted,
				fileIds: uploadedFileIds(result),
			});
			return null;
		};
		render(<Probe orgId="org-a" />);
		expect(log[0]).toStrictEqual({ adopted: false, fileIds: [] });
		expect(log.at(-1)).toStrictEqual({ adopted: true, fileIds: ["file-a"] });
	});

	it("discards an upload that completes after another org was adopted", async () => {
		const resolveUpload = mockDeferredUpload();
		const { result, rerender } = renderAttachments({ orgId: "org-a" });
		act(() => {
			result.current.startUpload(new File(["x"], "x.txt"));
		});

		rerender({ orgId: "org-b" });
		await waitFor(() => {
			expect(result.current.organizationAdopted).toBe(true);
		});
		await act(async () => {
			resolveUpload({ id: "file-x" });
		});

		expect(uploadedFileIds(result.current)).toStrictEqual([]);
		expect(
			localStorage.getItem(persistedAttachmentsStorage.key) ?? "",
		).not.toContain("file-x");
	});

	it("discards an upload that spans an org round trip", async () => {
		const resolveUpload = mockDeferredUpload();
		const { result, rerender } = renderAttachments({ orgId: "org-a" });
		act(() => {
			result.current.startUpload(new File(["x"], "x.txt"));
		});

		// After A -> B -> A, only the adoption epoch distinguishes the stale upload.
		rerender({ orgId: "org-b" });
		await waitFor(() => {
			expect(result.current.organizationAdopted).toBe(true);
		});
		rerender({ orgId: "org-a" });
		await waitFor(() => {
			expect(result.current.organizationAdopted).toBe(true);
		});
		await act(async () => {
			resolveUpload({ id: "file-x" });
		});

		expect(uploadedFileIds(result.current)).toStrictEqual([]);
		expect(
			localStorage.getItem(persistedAttachmentsStorage.key) ?? "",
		).not.toContain("file-x");
	});

	it("hides attachments when authorization leaves no org", async () => {
		localStorage.setItem(
			persistedAttachmentsStorage.key,
			JSON.stringify([persistEntry("file-a", "a.txt", "org-a")]),
		);
		const { result, rerender } = renderAttachments({ orgId: "org-a" });
		await waitFor(() => {
			expect(uploadedFileIds(result.current)).toStrictEqual(["file-a"]);
		});

		rerender({ orgId: undefined });
		expect(result.current.attachments).toStrictEqual([]);
		expect(uploadedFileIds(result.current)).toStrictEqual([]);
		expect(localStorage.getItem(persistedAttachmentsStorage.key)).toContain(
			"file-a",
		);

		rerender({ orgId: "org-a" });
		await waitFor(() => {
			expect(uploadedFileIds(result.current)).toStrictEqual(["file-a"]);
		});
	});

	it("does not prune storage during a render that never commits", () => {
		localStorage.setItem(
			persistedAttachmentsStorage.key,
			JSON.stringify([persistEntry("file-a", "a.txt", "org-a")]),
		);
		// Suspending after the hook runs simulates a render React abandons before commit.
		const Suspender: FC<{ orgId: string }> = ({ orgId }) => {
			useFileAttachments(orgId, { persist: true });
			throw new Promise(() => {});
		};
		render(
			<Suspense fallback={null}>
				<Suspender orgId="org-b" />
			</Suspense>,
		);
		expect(localStorage.getItem(persistedAttachmentsStorage.key)).toContain(
			"file-a",
		);
	});
});
