import { waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockPreviewSignInResponse } from "#/testHelpers/chatEntities";
import { MockWorkspace } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import PreviewOpenPage from "./PreviewOpenPage";

afterEach(() => {
	vi.restoreAllMocks();
});

describe("PreviewOpenPage", () => {
	it("signs in to the preview that the link names", async () => {
		vi.spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(
			MockWorkspace,
		);
		const signIn = vi
			.spyOn(API.experimental, "signInToPreview")
			.mockResolvedValue(MockPreviewSignInResponse);
		const search = new URLSearchParams({
			owner: "alice",
			workspace: "dev",
			agent: "main",
			slug: "preview--3000",
			path: "/docs?page=2",
		});

		renderWithAuth(<PreviewOpenPage />, {
			path: "/previews/open",
			route: `/previews/open?${search}`,
		});

		await waitFor(() =>
			expect(signIn).toHaveBeenCalledWith(
				expect.objectContaining({
					workspace_id: MockWorkspace.id,
					agent_name: "main",
					preview_slug: "preview--3000",
					url_path: "/docs?page=2",
				}),
			),
		);
	});
});
