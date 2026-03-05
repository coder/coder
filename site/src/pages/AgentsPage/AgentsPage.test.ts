import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { emptyInputStorageKey, useCreatePageDraft } from "./AgentsPage";

describe("useCreatePageDraft", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	it("reads the initial value from localStorage", () => {
		localStorage.setItem(emptyInputStorageKey, "saved draft");

		const { result, unmount } = renderHook(() => useCreatePageDraft());

		expect(result.current.initialInputValue).toBe("saved draft");
		expect(result.current.inputValueRef.current).toBe("saved draft");
		unmount();
	});

	it("returns empty string when localStorage has no draft", () => {
		const { result, unmount } = renderHook(() => useCreatePageDraft());

		expect(result.current.initialInputValue).toBe("");
		expect(result.current.inputValueRef.current).toBe("");
		unmount();
	});

	it("writes content to localStorage via handleContentChange", () => {
		const { result, unmount } = renderHook(() => useCreatePageDraft());

		act(() => {
			result.current.handleContentChange("work in progress");
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBe(
			"work in progress",
		);
		expect(result.current.inputValueRef.current).toBe("work in progress");
		unmount();
	});

	it("removes the draft key when handleContentChange receives empty string", () => {
		localStorage.setItem(emptyInputStorageKey, "old draft");
		const { result, unmount } = renderHook(() => useCreatePageDraft());

		act(() => {
			result.current.handleContentChange("");
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmount();
	});

	it("clears the draft from localStorage when markSent is called", () => {
		localStorage.setItem(emptyInputStorageKey, "draft to clear");
		const { result, unmount } = renderHook(() => useCreatePageDraft());

		expect(localStorage.getItem(emptyInputStorageKey)).toBe(
			"draft to clear",
		);

		act(() => {
			result.current.markSent();
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmount();
	});

	it("does not re-persist the draft after markSent is called", () => {
		localStorage.setItem(emptyInputStorageKey, "fix the bug");
		const { result, unmount } = renderHook(() => useCreatePageDraft());

		// Simulate handleSend: markSent clears the draft.
		act(() => {
			result.current.markSent();
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();

		// Simulate the Lexical ContentChangePlugin firing during
		// the re-render with the old content. Without the sentRef
		// guard this would re-persist the draft.
		act(() => {
			result.current.handleContentChange("fix the bug");
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		expect(result.current.inputValueRef.current).toBe("fix the bug");
		unmount();
	});
});
