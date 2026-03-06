import { MockTemplate, MockTemplateVersion } from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as apiModule from "api/api";

const { mockedPublishVersionName } = vi.hoisted(() => ({
	mockedPublishVersionName: "ai-published-version",
}));

vi.mock("./TemplateVersionEditor", () => ({
	TemplateVersionEditor: (props: {
		onPublishVersion: (data: {
			name?: string;
			message?: string;
			isActiveVersion?: boolean;
		}) => Promise<void>;
	}) => (
		<button
			type="button"
			onClick={() =>
				void props.onPublishVersion({
					name: mockedPublishVersionName,
					message: "Published from AI",
					isActiveVersion: true,
				})
			}
		>
			AI publish
		</button>
	),
}));

import TemplateVersionEditorPage from "./TemplateVersionEditorPage";

const { API } = apiModule;

describe("TemplateVersionEditorPage AI publish navigation", () => {
	beforeEach(() => {
		vi.restoreAllMocks();
	});

	it("navigates with React Router and preserves search params", async () => {
		const user = userEvent.setup();
		const replaceStateSpy = vi.spyOn(window.history, "replaceState");
		const publishedVersion = {
			...MockTemplateVersion,
			id: "published-version-id",
			name: mockedPublishVersionName,
		};
		const patchTemplateVersionSpy = vi
			.spyOn(API, "patchTemplateVersion")
			.mockResolvedValue(publishedVersion);
		const updateActiveTemplateVersionSpy = vi
			.spyOn(API, "updateActiveTemplateVersion")
			.mockResolvedValue({ message: "" });
		const { router } = renderWithAuth(<TemplateVersionEditorPage />, {
			route: `/templates/${MockTemplate.name}/versions/${MockTemplateVersion.name}/edit?path=myfile.tf`,
			path: "/templates/:template/versions/:version/edit",
			extraRoutes: [
				{
					path: "/templates/:templateId",
					element: <div></div>,
				},
			],
		});

		await user.click(await screen.findByRole("button", { name: "AI publish" }));

		await waitFor(() => {
			expect(patchTemplateVersionSpy).toHaveBeenCalledWith(
				MockTemplateVersion.id,
				{
					message: "Published from AI",
					name: mockedPublishVersionName,
				},
			);
			expect(updateActiveTemplateVersionSpy).toHaveBeenCalledWith(
				MockTemplate.name,
				{
					id: MockTemplateVersion.id,
				},
			);
			expect(router.state.location.pathname).toBe(
				`/templates/${MockTemplate.name}/versions/${mockedPublishVersionName}/edit`,
			);
		});
		expect(router.state.location.search).toBe("?path=myfile.tf");
		expect(replaceStateSpy).not.toHaveBeenCalled();
	});
});
