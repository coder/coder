import { render, renderHook, waitFor } from "@testing-library/react";
import { type FC, Suspense } from "react";
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

const renderAttachments = (initialProps: { orgId: string | undefined }) =>
	renderHook(
		({ orgId }: { orgId: string | undefined }) =>
			useFileAttachments(orgId, { persist: true }),
		{ initialProps },
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
		const { result, rerender } = renderAttachments({ orgId: undefined });

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
		const { result, rerender } = renderAttachments({ orgId: "org-a" });
		expect(uploadedFileIds(result.current)).toStrictEqual(["file-a"]);
		rerender({ orgId: "org-b" });
		await waitFor(() => {
			expect(uploadedFileIds(result.current)).toStrictEqual([]);
		});
		expect(
			localStorage.getItem(persistedAttachmentsStorageKey) ?? "",
		).not.toContain("file-a");
	});

	it("never exposes the previous org's file IDs in any render", async () => {
		localStorage.setItem(
			persistedAttachmentsStorageKey,
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

	it("does not prune storage during a render that never commits", () => {
		localStorage.setItem(
			persistedAttachmentsStorageKey,
			JSON.stringify([persistEntry("file-a", "a.txt", "org-a")]),
		);
		// Suspending after the hook call abandons the render before
		// commit, the same discard concurrent rendering can perform.
		const Suspender: FC<{ orgId: string }> = ({ orgId }) => {
			useFileAttachments(orgId, { persist: true });
			throw new Promise(() => {});
		};
		render(
			<Suspense fallback={null}>
				<Suspender orgId="org-b" />
			</Suspense>,
		);
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toContain(
			"file-a",
		);
	});
});
