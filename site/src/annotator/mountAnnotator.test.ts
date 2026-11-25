import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { annotatorHostId, mountAnnotator } from "./mountAnnotator";
import {
	type AnnotationSubmission,
	maxFieldLength,
	parseAnnotatorToHostMessage,
} from "./protocol";

const handles: ReturnType<typeof mountAnnotator>[] = [];

function mount(
	options: Omit<Parameters<typeof mountAnnotator>[0], "document">,
) {
	const handle = mountAnnotator({ document, ...options });
	handles.push(handle);
	return handle;
}

function overlay(): ShadowRoot {
	const host = Array.from(document.body.children).find(
		(element) => element.id === annotatorHostId && element.shadowRoot,
	);
	if (!host?.shadowRoot) {
		throw new Error("annotator host is not mounted");
	}
	return host.shadowRoot;
}

function overlayRegion(role: "dialog" | "toolbar"): HTMLElement {
	const region = Array.from(overlay().children).find(
		(element) => element.getAttribute("role") === role,
	);
	if (!(region instanceof HTMLElement)) {
		throw new Error(`annotator ${role} is not mounted`);
	}
	return region;
}

async function submitComment(comment: string) {
	const controls = within(overlayRegion("dialog"));
	await userEvent.type(
		controls.getByRole("textbox", { name: "Annotation comment" }),
		comment,
	);
	await userEvent.click(controls.getByRole("button", { name: "Send" }));
}

beforeEach(() => {
	document.body.innerHTML = "";
	document.title = "App";
	window.history.replaceState(null, "", "/");
});

afterEach(() => {
	for (const handle of handles.splice(0)) {
		handle.destroy();
	}
	window.getSelection()?.removeAllRanges();
});

describe("mountAnnotator", () => {
	it("submits one comment per pick with a usable selector and no URL or form secrets", async () => {
		document.body.innerHTML = `
			<section aria-label="Profile">
				<h2>Profile</h2>
				<a id="1:reset" href="/reset?token=link-secret#fragment">Reset password</a>
				<input aria-label="Email" value="form-secret@example.com">
				<textarea aria-label="Notes">private-notes</textarea>
				<div contenteditable="true">private-draft</div>
			</section>`;
		document.title = "t".repeat(maxFieldLength * 2);
		window.history.replaceState(
			null,
			"",
			"/profile?token=page-secret#fragment",
		);
		const onSubmit = vi.fn<(submission: AnnotationSubmission) => void>();
		const handle = mount({ onSubmit });
		handle.setPicking(true);

		await userEvent.click(screen.getByRole("link", { name: "Reset password" }));
		await submitComment(" Reset link ");
		window.history.replaceState(
			null,
			"",
			`/${"p".repeat(maxFieldLength * 2)}?token=page-secret`,
		);
		await userEvent.click(screen.getByRole("region", { name: "Profile" }));
		await submitComment("Update profile");

		expect(onSubmit).toHaveBeenCalledTimes(2);
		const first = onSubmit.mock.calls[0]?.[0];
		const second = onSubmit.mock.calls[1]?.[0];
		expect(first?.annotations).toHaveLength(1);
		expect(second?.annotations).toHaveLength(1);
		expect(first?.annotations[0]).toMatchObject({
			comment: "Reset link",
			element: {
				tag: "a",
				selector: "#\\31 \\:reset",
				openingTag: '<a id="1:reset" href="/reset">',
			},
		});
		expect(second?.annotations[0]).toMatchObject({
			comment: "Update profile",
			element: { tag: "section", text: "Profile Reset password" },
		});
		expect(first?.page).toMatchObject({
			url: `${window.location.origin}/profile`,
			title: "t".repeat(maxFieldLength),
		});
		expect(second?.page.url).toHaveLength(maxFieldLength);
		expect(JSON.stringify(onSubmit.mock.calls)).not.toMatch(
			/page-secret|link-secret|form-secret|private-notes|private-draft/,
		);
		for (const [submission] of onSubmit.mock.calls) {
			expect(
				parseAnnotatorToHostMessage({
					type: "coder-annotator:submit",
					...submission,
				}),
			).toMatchObject({
				type: "coder-annotator:submit",
				annotations: submission.annotations,
			});
		}
	});

	it("intercepts page clicks while picking but lets its own controls work", async () => {
		document.body.innerHTML = '<button type="button">Save page</button>';
		const pageButton = screen.getByRole("button", { name: "Save page" });
		const pageClick = vi.fn();
		const pagePointerDown = vi.fn();
		pageButton.addEventListener("click", pageClick);
		pageButton.addEventListener("pointerdown", pagePointerDown);
		const onSubmit = vi.fn();
		const onStateChange = vi.fn();
		const handle = mount({ onSubmit, onStateChange });
		await userEvent.click(
			within(overlayRegion("toolbar")).getByRole("button", {
				name: "Stop annotating",
				hidden: true,
			}),
		);
		expect(handle.getState()).toEqual({ picking: false });
		expect(onStateChange).not.toHaveBeenCalledWith({ picking: true });
		handle.setPicking(true);

		await userEvent.click(pageButton);
		expect(pageClick).not.toHaveBeenCalled();
		expect(pagePointerDown).not.toHaveBeenCalled();
		await submitComment("Change this");
		expect(onSubmit).toHaveBeenCalledTimes(1);
		expect(handle.getState()).toEqual({ picking: true });

		await userEvent.click(
			within(overlayRegion("toolbar")).getByRole("button", {
				name: "Stop annotating",
			}),
		);
		expect(handle.getState()).toEqual({ picking: false });
		await userEvent.click(pageButton);
		expect(pageClick).toHaveBeenCalledTimes(1);
		expect(pagePointerDown).toHaveBeenCalledTimes(1);
	});

	it("cancels a keyboard-picked draft before stopping and restores focus", async () => {
		document.body.innerHTML = '<button type="button">Save page</button>';
		const pageButton = screen.getByRole("button", { name: "Save page" });
		pageButton.focus();
		const onSubmit = vi.fn();
		const onStateChange = vi.fn();
		const handle = mount({ onSubmit, onStateChange });
		handle.setPicking(true);
		await userEvent.keyboard("{Enter}");
		await userEvent.type(
			within(overlayRegion("dialog")).getByRole("textbox", {
				name: "Annotation comment",
			}),
			"Unsaved",
		);
		await userEvent.keyboard("{Escape}");
		expect(handle.getState()).toEqual({ picking: true });
		expect(pageButton).toHaveFocus();
		await userEvent.keyboard(" ");
		await userEvent.click(
			within(overlayRegion("dialog")).getByRole("button", { name: "Cancel" }),
		);
		expect(handle.getState()).toEqual({ picking: true });
		expect(pageButton).toHaveFocus();
		await userEvent.keyboard("{Escape}");
		expect(handle.getState()).toEqual({ picking: false });
		expect(onStateChange).toHaveBeenLastCalledWith({ picking: false });
		handle.setPicking(true);
		await userEvent.click(
			within(overlayRegion("toolbar")).getByRole("button", {
				name: "Stop annotating",
			}),
		);
		expect(pageButton).toHaveFocus();
		expect(onSubmit).not.toHaveBeenCalled();
	});

	it("replaces its own mount without removing a page-owned host id or leaving click interceptors", async () => {
		const pageHost = document.createElement("div");
		pageHost.id = annotatorHostId;
		const pageButton = document.createElement("button");
		pageButton.textContent = "Save page";
		document.body.append(pageHost, pageButton);
		const pageClick = vi.fn();
		pageButton.addEventListener("click", pageClick);
		const firstSubmit = vi.fn();
		const first = mount({ onSubmit: firstSubmit });
		first.setPicking(true);
		const second = mount({ onSubmit: vi.fn() });

		expect(first.getState()).toEqual({ picking: false });
		expect(pageHost.isConnected).toBe(true);
		await userEvent.click(screen.getByRole("button", { name: "Save page" }));
		expect(pageClick).toHaveBeenCalledTimes(1);
		expect(firstSubmit).not.toHaveBeenCalled();
		second.setPicking(true);
		await userEvent.click(screen.getByRole("button", { name: "Save page" }));
		expect(pageClick).toHaveBeenCalledTimes(1);
		second.destroy();
		await userEvent.click(screen.getByRole("button", { name: "Save page" }));
		expect(pageClick).toHaveBeenCalledTimes(2);
		expect(pageHost.isConnected).toBe(true);
	});
});
