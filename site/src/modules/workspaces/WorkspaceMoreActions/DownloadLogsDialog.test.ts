import { describe, expect, it } from "vitest";
import { humanBlobSize } from "./DownloadLogsDialog";

describe("humanBlobSize", () => {
	it("formats bytes without decimals", () => {
		expect(humanBlobSize(200)).toBe("200 B");
	});

	it("promotes 1024 bytes to 1 KB", () => {
		expect(humanBlobSize(1024)).toBe("1 KB");
	});

	it("formats fractional kilobytes with up to 2 decimals", () => {
		expect(humanBlobSize(1491)).toBe("1.46 KB");
		expect(humanBlobSize(4472)).toBe("4.37 KB");
	});

	it("formats larger units", () => {
		expect(humanBlobSize(1024 * 1024)).toBe("1 MB");
		expect(humanBlobSize(1024 * 1024 * 1.5)).toBe("1.5 MB");
	});
});
