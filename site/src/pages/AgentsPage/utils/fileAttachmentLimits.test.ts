import { describe, expect, it } from "vitest";
import { mockApiError } from "#/testHelpers/entities";
import { formatAgentAttachmentUploadError } from "./fileAttachmentLimits";

const axiosApiError = (message: string, detail?: string) =>
	Object.assign(
		new Error("Request failed with status code 409"),
		mockApiError({ message, detail }),
	);

describe("formatAgentAttachmentUploadError", () => {
	it("returns the server message alone when there is no detail", () => {
		expect(
			formatAgentAttachmentUploadError(
				axiosApiError(
					"Workspace is stopped. Start the workspace before uploading files.",
				),
			),
		).toBe("Workspace is stopped. Start the workspace before uploading files.");
	});

	it("does not double punctuation before the detail", () => {
		expect(
			formatAgentAttachmentUploadError(
				axiosApiError(
					"Failed to upload file to workspace agent.",
					"The workspace agent could not be reached.",
				),
			),
		).toBe(
			"Failed to upload file to workspace agent. The workspace agent could not be reached.",
		);
	});

	it("adds a period between an unpunctuated message and the detail", () => {
		expect(
			formatAgentAttachmentUploadError(
				axiosApiError("Upload rejected", "File type not allowed."),
			),
		).toBe("Upload rejected. File type not allowed.");
	});

	it("omits the developer console hint for non-API errors", () => {
		expect(formatAgentAttachmentUploadError(new Error("Network Error"))).toBe(
			"Network Error",
		);
	});
});
