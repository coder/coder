import { beforeEach, describe, expect, it, vi } from "vitest";

describe("defaultDocsUrl", () => {
	beforeEach(() => {
		// Reset module-level caches (CACHED_DOCS_URL in docs.ts and
		// CACHED_BUILD_INFO in buildInfo.ts) by forcing fresh imports.
		vi.resetModules();
		// Clean up meta tags from previous tests so each case starts fresh.
		document.querySelector('meta[property="docs-url"]')?.remove();
		document.querySelector('meta[property="build-info"]')?.remove();
	});

	function setBuildInfoVersion(version: string) {
		const meta = document.createElement("meta");
		meta.setAttribute("property", "build-info");
		meta.setAttribute("content", JSON.stringify({ version }));
		document.head.appendChild(meta);
	}

	async function getDocsUrl(path: string): Promise<string> {
		// Dynamic import so we get a fresh module with cleared caches.
		const { docs } = await import("./docs");
		return docs(path);
	}

	it("should preserve RC prerelease and strip build metadata", async () => {
		setBuildInfoVersion("v2.32.0-rc.1+abc123");
		const url = await getDocsUrl("/admin/users");
		expect(url).toBe("https://coder.com/docs/@v2.32.0-rc.1/admin/users");
	});

	it("should preserve RC prerelease when no build metadata present", async () => {
		setBuildInfoVersion("v2.32.0-rc.0");
		const url = await getDocsUrl("/admin/users");
		expect(url).toBe("https://coder.com/docs/@v2.32.0-rc.0/admin/users");
	});

	it("should strip devel suffix and build metadata", async () => {
		setBuildInfoVersion("v2.16.0-devel+683a720");
		const url = await getDocsUrl("/admin/users");
		expect(url).toBe("https://coder.com/docs/@v2.16.0/admin/users");
	});

	it("should strip build metadata from release version", async () => {
		setBuildInfoVersion("v2.16.0+683a720");
		const url = await getDocsUrl("/admin/users");
		expect(url).toBe("https://coder.com/docs/@v2.16.0/admin/users");
	});

	it("should strip bare devel suffix with no build metadata", async () => {
		setBuildInfoVersion("v2.32.0-devel");
		const url = await getDocsUrl("/admin/users");
		expect(url).toBe("https://coder.com/docs/@v2.32.0/admin/users");
	});

	it("should use plain release version as-is", async () => {
		setBuildInfoVersion("v2.16.0");
		const url = await getDocsUrl("/admin/users");
		expect(url).toBe("https://coder.com/docs/@v2.16.0/admin/users");
	});

	it("should produce unversioned URL for v0.0.0 dev builds", async () => {
		setBuildInfoVersion("v0.0.0-devel+abc123");
		const url = await getDocsUrl("/admin/users");
		expect(url).toBe("https://coder.com/docs/admin/users");
	});
});
