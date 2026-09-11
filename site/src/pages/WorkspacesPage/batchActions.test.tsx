import { act, renderHook, waitFor } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { toast } from "sonner";
import { API } from "#/api/api";
import { MockWorkspace } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { useBatchActions } from "./batchActions";

const createWrapper = (): FC<PropsWithChildren> => {
	const queryClient = createTestQueryClient();
	return ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
};

describe("useBatchActions", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});
	it("does not toast when stopping workspaces fails", async () => {
		const error = new Error("workspace is already busy with a build");
		vi.spyOn(API, "stopWorkspace").mockRejectedValue(error);
		const toastError = vi.spyOn(toast, "error");
		const onSuccess = vi.fn();
		const { result } = renderHook(() => useBatchActions({ onSuccess }), {
			wrapper: createWrapper(),
		});

		act(() => {
			result.current.stop([MockWorkspace]);
		});

		await waitFor(() => {
			expect(result.current.stopError).toBe(error);
		});
		expect(toastError).not.toHaveBeenCalled();
		expect(onSuccess).not.toHaveBeenCalled();
	});

	it("does not toast when deleting workspaces fails", async () => {
		const error = new Error("workspace cannot be deleted");
		vi.spyOn(API, "deleteWorkspace").mockRejectedValue(error);
		const toastError = vi.spyOn(toast, "error");
		const onSuccess = vi.fn();
		const { result } = renderHook(() => useBatchActions({ onSuccess }), {
			wrapper: createWrapper(),
		});

		act(() => {
			result.current.delete([MockWorkspace]);
		});

		await waitFor(() => {
			expect(result.current.deleteError).toBe(error);
		});
		expect(toastError).not.toHaveBeenCalled();
		expect(onSuccess).not.toHaveBeenCalled();
	});
});
