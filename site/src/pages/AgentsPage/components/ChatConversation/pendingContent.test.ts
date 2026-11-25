import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { buildPendingContent } from "./pendingContent";

describe("buildPendingContent", () => {
	it("preserves image MIME types for newly attached files", () => {
		const content = buildPendingContent({
			requestContent: [{ type: "file", file_id: "image-1" }],
			attachmentMediaTypes: new Map([["image-1", "image/png"]]),
		});

		expect(content).toEqual([
			{ type: "file", file_id: "image-1", media_type: "image/png" },
		]);
	});

	it("reuses existing file parts before local attachment metadata", () => {
		const existingFilePart: TypesGen.ChatFilePart = {
			type: "file",
			file_id: "existing-1",
			media_type: "image/jpeg",
		};
		const content = buildPendingContent({
			requestContent: [{ type: "file", file_id: "existing-1" }],
			originalContent: [existingFilePart],
			attachmentMediaTypes: new Map([["existing-1", "text/plain"]]),
		});

		expect(content).toEqual([existingFilePart]);
	});

	it("keeps skill invocations as skill parts", () => {
		const skillPart: TypesGen.ChatSkillInvocationPart = {
			type: "skill-invocation",
			skill_source: "project",
			skill_name: "triage",
		};
		const content = buildPendingContent({
			requestContent: [skillPart],
		});

		expect(content).toEqual([skillPart]);
	});
});
