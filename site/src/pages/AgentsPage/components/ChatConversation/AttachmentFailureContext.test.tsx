import { act, renderHook } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AttachmentFailure } from "../../utils/chatAttachments";
import {
	AttachmentFailureProvider,
	useAttachmentFailure,
} from "./AttachmentFailureContext";

const wrapper: FC<PropsWithChildren> = ({ children }) => (
	<AttachmentFailureProvider>{children}</AttachmentFailureProvider>
);

describe("useAttachmentFailure", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("deduplicates concurrent probes by file ID", async () => {
		const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response(null, {
				status: 404,
			}),
		);
		const { result } = renderHook(() => useAttachmentFailure(), { wrapper });
		let failures: AttachmentFailure[] = [];

		await act(async () => {
			failures = await Promise.all([
				result.current.probeFailure("file-1", "/api/files/file-1"),
				result.current.probeFailure("file-1", "/api/files/file-1"),
			]);
		});

		expect(fetchSpy).toHaveBeenCalledTimes(1);
		expect(failures).toEqual([{ kind: "expired" }, { kind: "expired" }]);
		expect(result.current.getFailure("file-1")).toEqual({ kind: "expired" });
	});

	it("retries non-expired probe failures", async () => {
		const fetchSpy = vi
			.spyOn(globalThis, "fetch")
			.mockResolvedValueOnce(
				new Response("temporary outage", {
					status: 502,
					statusText: "Bad Gateway",
				}),
			)
			.mockResolvedValueOnce(
				new Response(null, {
					status: 404,
				}),
			);
		const { result } = renderHook(() => useAttachmentFailure(), { wrapper });
		let firstFailure: AttachmentFailure | undefined;
		let secondFailure: AttachmentFailure | undefined;

		await act(async () => {
			firstFailure = await result.current.probeFailure(
				"file-1",
				"/api/files/file-1",
			);
		});

		expect(firstFailure).toEqual({
			kind: "failed",
			detail: "502 Bad Gateway",
		});
		expect(result.current.getFailure("file-1")).toBeUndefined();

		await act(async () => {
			secondFailure = await result.current.probeFailure(
				"file-1",
				"/api/files/file-1",
			);
		});

		expect(fetchSpy).toHaveBeenCalledTimes(2);
		expect(secondFailure).toEqual({ kind: "expired" });
		expect(result.current.getFailure("file-1")).toEqual({ kind: "expired" });
	});
});
