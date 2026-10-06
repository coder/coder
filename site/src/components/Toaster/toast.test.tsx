import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "./Toaster";
import { toast } from "./toast";

// Sonner's built-in auto-dismiss delay when no duration is configured.
const SONNER_DEFAULT_DURATION_MS = 4_000;

const advance = (ms: number) => act(() => vi.advanceTimersByTimeAsync(ms));

describe("toast", () => {
	beforeEach(() => {
		vi.useFakeTimers({ shouldAdvanceTime: true });
		render(<Toaster />);
	});

	afterEach(() => {
		act(() => toast.dismiss());
		vi.useRealTimers();
	});

	it("keeps error toasts visible until the user dismisses them", async () => {
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		act(() => {
			toast.error('Failed to open "Cursor Desktop".', {
				description: "The app must be installed first.",
			});
		});
		expect(
			await screen.findByText('Failed to open "Cursor Desktop".'),
		).toBeInTheDocument();

		await advance(SONNER_DEFAULT_DURATION_MS * 10);
		expect(
			screen.getByText('Failed to open "Cursor Desktop".'),
		).toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: "Close toast" }));
		await advance(1_000);
		await waitFor(() =>
			expect(
				screen.queryByText('Failed to open "Cursor Desktop".'),
			).not.toBeInTheDocument(),
		);
	});

	it("keeps the error toast of a rejected promise visible", async () => {
		act(() => {
			toast.promise(Promise.reject(new Error("already a member")), {
				loading: "Adding member...",
				error: (err) => ({
					message: "Failed to add member.",
					description: err instanceof Error ? err.message : undefined,
				}),
			});
		});
		expect(
			await screen.findByText("Failed to add member."),
		).toBeInTheDocument();

		await advance(SONNER_DEFAULT_DURATION_MS * 10);
		expect(screen.getByText("Failed to add member.")).toBeInTheDocument();
		expect(screen.getByText("already a member")).toBeInTheDocument();
	});

	it("still auto-dismisses success toasts", async () => {
		act(() => {
			toast.success("Member added.");
		});
		expect(await screen.findByText("Member added.")).toBeInTheDocument();

		await advance(SONNER_DEFAULT_DURATION_MS * 2);
		await waitFor(() =>
			expect(screen.queryByText("Member added.")).not.toBeInTheDocument(),
		);
	});
});
