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
	const overrideNavigator = (key: string, value: unknown) => {
		Object.defineProperty(navigator, key, {
			value,
			configurable: true,
		});
		overriddenNavigatorKeys.add(key);
	};

	const iPhoneUserAgent =
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15";

	const enterIOSStandalonePWA = () => {
		overrideNavigator("userAgent", iPhoneUserAgent);
		overrideNavigator("standalone", true);
	};

	const target = {
		href: "/api/experimental/chats/files/file-1",
		fileName: "01-agents-list.png",
		mediaType: "image/png",
	};

	afterEach(() => {
		for (const key of overriddenNavigatorKeys) {
			Reflect.deleteProperty(navigator, key);
		}
		overriddenNavigatorKeys.clear();
		vi.restoreAllMocks();
		vi.mocked(toast.error).mockClear();
	});

	it("keeps the native anchor download outside iOS", () => {
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		const event = { preventDefault: vi.fn() };

		expect(handleAttachmentDownloadClick(event, target)).toBeUndefined();
		expect(event.preventDefault).not.toHaveBeenCalled();
		expect(open).not.toHaveBeenCalled();
	});

	it("keeps the native anchor download in the iOS browser", () => {
		overrideNavigator("userAgent", iPhoneUserAgent);
		const event = { preventDefault: vi.fn() };

		expect(handleAttachmentDownloadClick(event, target)).toBeUndefined();
		expect(event.preventDefault).not.toHaveBeenCalled();
	});

	it("shares the attachment via the share sheet in an iOS standalone PWA", async () => {
		enterIOSStandalonePWA();
		const share = vi.fn().mockResolvedValue(undefined);
		overrideNavigator("share", share);
		overrideNavigator("canShare", vi.fn().mockReturnValue(true));
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response(new Blob(["png-bytes"], { type: "image/png" }), {
				status: 200,
			}),
		);
		const event = { preventDefault: vi.fn() };

		await handleAttachmentDownloadClick(event, target);

		expect(event.preventDefault).toHaveBeenCalled();
		expect(globalThis.fetch).toHaveBeenCalledWith(target.href);
		expect(open).not.toHaveBeenCalled();
		expect(share).toHaveBeenCalledTimes(1);
		const shared: { files: File[] } = share.mock.calls[0][0];
		expect(shared.files).toHaveLength(1);
		expect(shared.files[0].name).toBe("01-agents-list.png");
		expect(shared.files[0].type).toBe("image/png");
	});

	it("intercepts on iPadOS reporting a macOS user agent", () => {
		overrideNavigator(
			"userAgent",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15",
		);
		overrideNavigator("maxTouchPoints", 5);
		overrideNavigator("standalone", true);
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		const event = { preventDefault: vi.fn() };

		expect(handleAttachmentDownloadClick(event, target)).toBeUndefined();
		expect(event.preventDefault).toHaveBeenCalled();
		expect(open).toHaveBeenCalled();
	});

	it("falls back to a dismissible tab when file sharing is unavailable", () => {
		enterIOSStandalonePWA();
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		const fetchSpy = vi.spyOn(globalThis, "fetch");
		const event = { preventDefault: vi.fn() };

		expect(handleAttachmentDownloadClick(event, target)).toBeUndefined();
		expect(event.preventDefault).toHaveBeenCalled();
		expect(open).toHaveBeenCalledWith(target.href, "_blank", "noopener");
		expect(fetchSpy).not.toHaveBeenCalled();
	});

	it("stays quiet when the user dismisses the share sheet", async () => {
		enterIOSStandalonePWA();
		overrideNavigator(
			"share",
			vi.fn().mockRejectedValue(new DOMException("canceled", "AbortError")),
		);
		overrideNavigator("canShare", vi.fn().mockReturnValue(true));
		vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response(new Blob(["png-bytes"], { type: "image/png" })),
		);
		const event = { preventDefault: vi.fn() };

		await handleAttachmentDownloadClick(event, target);

		expect(toast.error).not.toHaveBeenCalled();
	});

	it("shows an error toast without a late popup when the download fetch fails", async () => {
		enterIOSStandalonePWA();
		const share = vi.fn().mockResolvedValue(undefined);
		overrideNavigator("share", share);
		overrideNavigator("canShare", vi.fn().mockReturnValue(true));
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response("nope", { status: 503 }),
		);
		const event = { preventDefault: vi.fn() };

		await handleAttachmentDownloadClick(event, target);

		expect(share).not.toHaveBeenCalled();
		expect(open).not.toHaveBeenCalled();
		expect(toast.error).toHaveBeenCalledWith(
			"Couldn't download 01-agents-list.png",
			{ description: "HTTP 503" },
		);
	});

	it("offers a fresh-gesture retry when user activation expired during the fetch", async () => {
		enterIOSStandalonePWA();
		const share = vi
			.fn()
			.mockRejectedValueOnce(
				new DOMException("activation expired", "NotAllowedError"),
			)
			.mockResolvedValue(undefined);
		overrideNavigator("share", share);
		overrideNavigator("canShare", vi.fn().mockReturnValue(true));
		vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response(new Blob(["png-bytes"], { type: "image/png" })),
		);
		const event = { preventDefault: vi.fn() };

		await handleAttachmentDownloadClick(event, target);

		// DownloadInIOSStandaloneRecoversExpiredActivation covers the Save click
		// and retry through the real toast UI.
		expect(share).toHaveBeenCalledTimes(1);
		expect(toast.error).toHaveBeenCalledWith(
			"Couldn't download 01-agents-list.png",
			expect.objectContaining({
				description: "The file is ready to save.",
				action: expect.objectContaining({ label: "Save" }),
			}),
		);
	});

	it("reports permanent share failures without a retry action", async () => {
		enterIOSStandalonePWA();
		overrideNavigator(
			"share",
			vi.fn().mockRejectedValue(new DOMException("share failed", "DataError")),
		);
		overrideNavigator("canShare", vi.fn().mockReturnValue(true));
		vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response(new Blob(["png-bytes"], { type: "image/png" })),
		);
		const event = { preventDefault: vi.fn() };

		await handleAttachmentDownloadClick(event, target);

		expect(toast.error).toHaveBeenCalledTimes(1);
		expect(vi.mocked(toast.error).mock.calls[0][1]).not.toHaveProperty(
			"action",
		);
	});

	it("offers a tab fallback when the fetched file turns out unshareable", async () => {
		enterIOSStandalonePWA();
		const share = vi.fn().mockResolvedValue(undefined);
		overrideNavigator("share", share);
		overrideNavigator(
			"canShare",
			vi
				.fn<(data: { files: File[] }) => boolean>()
				.mockImplementation(({ files }) => files[0].size <= 1),
		);
		vi.spyOn(globalThis, "fetch").mockResolvedValue(
			new Response(new Blob(["png-bytes"], { type: "image/png" })),
		);
		const open = vi.spyOn(window, "open").mockReturnValue(null);
		const event = { preventDefault: vi.fn() };

		await handleAttachmentDownloadClick(event, target);

		expect(share).not.toHaveBeenCalled();
		expect(open).not.toHaveBeenCalled();
		expect(toast.error).toHaveBeenCalledWith(
			"Couldn't download 01-agents-list.png",
			expect.objectContaining({
				description: "This file cannot be shared on this device.",
				action: expect.objectContaining({ label: "Open" }),
			}),
		);
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
