import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import {
	emptyInputStorageKey,
	useEmptyStateDraft,
} from "./components/AgentCreateForm";

describe("useEmptyStateDraft", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	const renderDraft = () => renderHook(() => useEmptyStateDraft());

	it("reads the initial value from localStorage", () => {
		localStorage.setItem(emptyInputStorageKey, "saved draft");

		const { result, unmount } = renderDraft();

		expect(result.current.initialInputValue).toBe("saved draft");
		expect(result.current.getCurrentContent()).toBe("saved draft");
		unmount();
	});

	it("returns empty string when localStorage has no draft", () => {
		const { result, unmount } = renderDraft();

		expect(result.current.initialInputValue).toBe("");
		expect(result.current.getCurrentContent()).toBe("");
		unmount();
	});

	it("writes content to localStorage via handleContentChange", () => {
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.handleContentChange("work in progress");
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBe("work in progress");
		expect(result.current.getCurrentContent()).toBe("work in progress");
		unmount();
	});

	it("removes the draft key when handleContentChange receives empty string", () => {
		localStorage.setItem(emptyInputStorageKey, "old draft");
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.handleContentChange("");
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmount();
	});

	it("clears the draft from localStorage when submitDraft is called", () => {
		localStorage.setItem(emptyInputStorageKey, "draft to clear");
		const { result, unmount } = renderDraft();

		expect(localStorage.getItem(emptyInputStorageKey)).toBe("draft to clear");

		act(() => {
			result.current.submitDraft();
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmount();
	});

	it("does not re-persist the draft after submitDraft is called", () => {
		localStorage.setItem(emptyInputStorageKey, "fix the bug");
		const { result, unmount } = renderDraft();

		// Simulate handleSend: submitDraft clears the draft.
		act(() => {
			result.current.submitDraft();
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();

		// Simulate the Lexical ContentChangePlugin firing during
		// the re-render with the old content. Without the sentRef
		// guard this would re-persist the draft.
		act(() => {
			result.current.handleContentChange("fix the bug");
		});

		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		expect(result.current.getCurrentContent()).toBe("fix the bug");
		unmount();
	});

	it("ignores all handleContentChange calls after submitDraft, even with new content", () => {
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.handleContentChange("original");
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBe("original");

		act(() => {
			result.current.submitDraft();
		});

		act(() => {
			result.current.handleContentChange("totally new content");
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmount();
	});

	it("returns empty draft when remounting after submitDraft", () => {
		localStorage.setItem(emptyInputStorageKey, "draft before send");
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.submitDraft();
		});
		unmount();

		// Simulate returning to the page.
		const { result: fresh, unmount: unmountFresh } = renderDraft();
		expect(fresh.current.initialInputValue).toBe("");
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmountFresh();
	});

	it("re-enables draft persistence after resetDraft", () => {
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.handleContentChange("attempt one");
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBe("attempt one");

		act(() => {
			result.current.submitDraft();
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();

		// Simulate error recovery — re-enable persistence.
		act(() => {
			result.current.resetDraft();
		});

		act(() => {
			result.current.handleContentChange("attempt two");
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBe("attempt two");
		unmount();
	});

	it("handles submitDraft being called twice without error", () => {
		localStorage.setItem(emptyInputStorageKey, "draft");
		const { result, unmount } = renderDraft();

		act(() => {
			result.current.submitDraft();
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();

		act(() => {
			result.current.submitDraft();
		});
		expect(localStorage.getItem(emptyInputStorageKey)).toBeNull();
		unmount();
	});
});
