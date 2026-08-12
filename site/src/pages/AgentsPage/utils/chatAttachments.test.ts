import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("sonner", () => ({
	toast: {
		error: vi.fn(),
	},
}));

import { toast } from "sonner";
import {
	handleAttachmentDownloadClick,
	isChatAttachmentFile,
	renameChatFileForUpload,
	sanitizeChatFileName,
} from "./chatAttachments";

describe("handleAttachmentDownloadClick", () => {
	const overriddenNavigatorKeys = new Set<string>();
	const iPhoneUserAgent =
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15";
	const target = {
		href: "/api/experimental/chats/files/file-1",
		fileName: "01-agents-list.png",
		mediaType: "image/png",
	};

	const overrideNavigator = (key: string, value: unknown) => {
		Object.defineProperty(navigator, key, { value, configurable: true });
		overriddenNavigatorKeys.add(key);
	};

	const enterIOSStandalonePWA = () => {
		overrideNavigator("userAgent", iPhoneUserAgent);
		overrideNavigator("standalone", true);
	};

	const mockFileSharing = (share: ReturnType<typeof vi.fn>) => {
		overrideNavigator("share", share);
		overrideNavigator(
			"canShare",
			vi.fn(() => true),
		);
	};

	const mockAttachmentFetch = (body = "png-bytes", mediaType = "image/png") =>
		vi
			.spyOn(globalThis, "fetch")
			.mockResolvedValue(
				new Response(new Blob([body], { type: mediaType }), { status: 200 }),
			);

	const click = (downloadTarget = target) => {
		const event = { preventDefault: vi.fn() };
		return {
			event,
			pending: handleAttachmentDownloadClick(event, downloadTarget),
		};
	};

	afterEach(() => {
		for (const key of overriddenNavigatorKeys) {
			Reflect.deleteProperty(navigator, key);
		}
		overriddenNavigatorKeys.clear();
		vi.restoreAllMocks();
		vi.mocked(toast.error).mockClear();
	});

	it.each([
		["outside iOS", () => {}],
		[
			"in the iOS browser",
			() => overrideNavigator("userAgent", iPhoneUserAgent),
		],
	])("keeps the native anchor download %s", (_label, setup) => {
		setup();
		const { event, pending } = click();

		expect(pending).toBeUndefined();
		expect(event.preventDefault).not.toHaveBeenCalled();
	});

	it("shares the fetched attachment in an iOS standalone PWA", async () => {
		enterIOSStandalonePWA();
		const share = vi.fn().mockResolvedValue(undefined);
		mockFileSharing(share);
		mockAttachmentFetch();

		const { event, pending } = click();
		await pending;

		expect(event.preventDefault).toHaveBeenCalled();
		expect(globalThis.fetch).toHaveBeenCalledWith(target.href);
		const shared: { files: File[] } = share.mock.calls[0][0];
		expect(shared.files).toHaveLength(1);
		expect(shared.files[0]).toMatchObject({
			name: "01-agents-list.png",
			type: "image/png",
		});
	});

	it("recognizes iPadOS with a macOS user agent", async () => {
		overrideNavigator(
			"userAgent",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15",
		);
		overrideNavigator("maxTouchPoints", 5);
		overrideNavigator("standalone", true);
		const share = vi.fn().mockResolvedValue(undefined);
		mockFileSharing(share);
		mockAttachmentFetch();

		const { event, pending } = click();
		await pending;

		expect(event.preventDefault).toHaveBeenCalled();
		expect(share).toHaveBeenCalledTimes(1);
	});

	it("keeps the native anchor when file sharing is unavailable", () => {
		enterIOSStandalonePWA();
		const fetchSpy = vi.spyOn(globalThis, "fetch");

		const { event, pending } = click();

		expect(pending).toBeUndefined();
		expect(event.preventDefault).not.toHaveBeenCalled();
		expect(fetchSpy).not.toHaveBeenCalled();
	});

	it("stays quiet when the user dismisses the share sheet", async () => {
		enterIOSStandalonePWA();
		mockFileSharing(
			vi.fn().mockRejectedValue(new DOMException("canceled", "AbortError")),
		);
		mockAttachmentFetch();

		await click().pending;

		expect(toast.error).not.toHaveBeenCalled();
	});

	it("shows the fetch failure", async () => {
		enterIOSStandalonePWA();
		const share = vi.fn().mockResolvedValue(undefined);
		mockFileSharing(share);
		vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response("nope", { status: 503 }),
		);

		await click().pending;

		expect(share).not.toHaveBeenCalled();
		expect(toast.error).toHaveBeenCalledWith(
			"Couldn't download 01-agents-list.png",
			{ description: "HTTP 503" },
		);
	});

	it("offers a Save retry when user activation expires", async () => {
		enterIOSStandalonePWA();
		const share = vi
			.fn()
			.mockRejectedValueOnce(
				new DOMException("activation expired", "NotAllowedError"),
			)
			.mockResolvedValue(undefined);
		mockFileSharing(share);
		mockAttachmentFetch();

		await click().pending;

		expect(toast.error).toHaveBeenCalledWith(
			"Couldn't download 01-agents-list.png",
			expect.objectContaining({
				description: "The file is ready to save.",
				action: expect.objectContaining({ label: "Save" }),
			}),
		);

		const action: unknown = vi.mocked(toast.error).mock.calls[0][1]?.action;
		if (
			action === null ||
			typeof action !== "object" ||
			!("onClick" in action) ||
			typeof action.onClick !== "function"
		) {
			throw new Error("expected the toast to carry a Save action");
		}
		action.onClick();
		expect(share).toHaveBeenCalledTimes(2);
		const first: { files: File[] } = share.mock.calls[0][0];
		const retry: { files: File[] } = share.mock.calls[1][0];
		expect(retry.files[0]).toBe(first.files[0]);
	});

	it("shows a plain failure toast after a permanent share failure", async () => {
		enterIOSStandalonePWA();
		mockFileSharing(vi.fn().mockRejectedValue(new Error("share failed")));
		mockAttachmentFetch();

		await click().pending;

		expect(toast.error).toHaveBeenCalledWith(
			"Couldn't download 01-agents-list.png",
			{ description: "share failed" },
		);
	});

	it("shares inline data without fetching", async () => {
		enterIOSStandalonePWA();
		const share = vi.fn().mockResolvedValue(undefined);
		mockFileSharing(share);
		const fetchSpy = vi.spyOn(globalThis, "fetch");

		await click({
			href: `data:image/png;base64,${btoa("png-bytes")}`,
			fileName: "inline.png",
			mediaType: "image/png",
		}).pending;

		expect(fetchSpy).not.toHaveBeenCalled();
		const shared: { files: File[] } = share.mock.calls[0][0];
		expect(shared.files[0]).toMatchObject({
			name: "inline.png",
			type: "image/png",
			size: "png-bytes".length,
		});
	});

	it("shows a decode error for corrupt inline data", async () => {
		enterIOSStandalonePWA();
		const share = vi.fn().mockResolvedValue(undefined);
		mockFileSharing(share);

		await click({
			href: "data:image/png;base64,%%%",
			fileName: "inline.png",
			mediaType: "image/png",
		}).pending;

		expect(share).not.toHaveBeenCalled();
		expect(toast.error).toHaveBeenCalledWith("Couldn't download inline.png", {
			description: "The attachment data could not be decoded.",
		});
	});
});

describe("isChatAttachmentFile", () => {
	it("accepts allowlisted MIME types", () => {
		const file = new File(["png"], "image.png", { type: "image/png" });

		expect(isChatAttachmentFile(file)).toBe(true);
	});

	it("accepts files with an empty MIME type", () => {
		const file = new File(["markdown"], "notes.md");

		expect(isChatAttachmentFile(file)).toBe(true);
	});

	it("accepts application/octet-stream files", () => {
		const file = new File(["unknown"], "attachment.bin", {
			type: "application/octet-stream",
		});

		expect(isChatAttachmentFile(file)).toBe(true);
	});

	it("rejects unsupported MIME types", () => {
		const file = new File(["zip"], "archive.zip", {
			type: "application/zip",
		});

		expect(isChatAttachmentFile(file)).toBe(false);
	});
});

describe("sanitizeChatFileName", () => {
	it.each([
		// Already safe.
		["clean.pdf", "clean.pdf"],
		// Spaces, parens collapsed into a single underscore each.
		["My Report (final).pdf", "My_Report_final_.pdf"],
		// `!` is kept; only `&` and the space become underscores.
		["weird & stuff!.txt", "weird_stuff!.txt"],
		// Path separators (forward and backslash) become underscores.
		["path/with\\slash.png", "path_with_slash.png"],
		// Leading dots/spaces/underscores are trimmed.
		["   .leading.dots.txt", "leading.dots.txt"],
		// Non-ASCII letters survive.
		["日本語のファイル.txt", "日本語のファイル.txt"],
		// Emoji survive.
		["🔥emoji🔥.png", "🔥emoji🔥.png"],
		// Control characters are stripped (replaced and trimmed).
		["\u0000\u0001\tcontrol.bin", "control.bin"],
		// Underscore-only collapses to empty then falls back to "file".
		["___", "file"],
		// Empty input falls back to "file".
		["", "file"],
		// Trailing problem characters are also trimmed.
		["foo!.pdf ", "foo!.pdf"],
	])("sanitizes %j to %j", (input, expected) => {
		expect(sanitizeChatFileName(input)).toBe(expected);
	});
});

describe("renameChatFileForUpload", () => {
	it("returns the same File reference when the name is already safe", () => {
		const file = new File(["png"], "clean.png", { type: "image/png" });

		// Identity matters: useFileAttachments keys preview-URL,
		// upload-state, and text-content Maps on the File object.
		expect(renameChatFileForUpload(file)).toBe(file);
	});

	it("returns a new File with a sanitized name when needed", () => {
		const file = new File(["pdf-bytes"], "My Report (final).pdf", {
			type: "application/pdf",
			lastModified: 1_700_000_000_000,
		});

		const renamed = renameChatFileForUpload(file);

		expect(renamed).not.toBe(file);
		expect(renamed.name).toBe("My_Report_final_.pdf");
		expect(renamed.type).toBe("application/pdf");
		expect(renamed.lastModified).toBe(1_700_000_000_000);
		// File size preserved; byte content is covered transitively by
		// the File constructor, and jsdom's Blob backing in this
		// project is not reliable enough for an explicit text() probe.
		expect(renamed.size).toBe(file.size);
	});
});
