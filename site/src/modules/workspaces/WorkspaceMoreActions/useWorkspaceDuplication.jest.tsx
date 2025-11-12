import * as M from "testHelpers/entities";
import {
	type GetLocationSnapshot,
	renderHookWithAuth,
} from "testHelpers/hooks";
import { act, waitFor } from "@testing-library/react";
import type { Workspace } from "api/typesGenerated";
import CreateWorkspacePage from "pages/CreateWorkspacePage/CreateWorkspacePage";
import { useWorkspaceDuplication } from "./useWorkspaceDuplication";

function render(workspace?: Workspace) {
	return renderHookWithAuth(
		({ workspace }) => useWorkspaceDuplication(workspace),
		{
			renderOptions: { initialProps: { workspace } },
			routingOptions: {
				extraRoutes: [
					{
						path: "/templates/:organization/:template/workspace",
						element: <CreateWorkspacePage />,
					},
					{
						path: "/templates/:template/workspace",
						element: <CreateWorkspacePage />,
					},
				],
			},
		},
	);
}

type RenderResult = Awaited<ReturnType<typeof render>>;
type RenderHookResult = RenderResult["result"];

async function performNavigation(
	result: RenderHookResult,
	getLocationSnapshot: GetLocationSnapshot,
) {
	await waitFor(() => expect(result.current.isDuplicationReady).toBe(true));
	act(() => result.current.duplicateWorkspace());

	const templateName = M.MockWorkspace.template_name;
	return waitFor(() => {
		const { pathname } = getLocationSnapshot();
		expect(pathname).toEqual(`/templates/${templateName}/workspace`);
	});
}

describe(`${useWorkspaceDuplication.name}`, () => {
	it("Will never be ready when there is no workspace passed in", async () => {
		const { result, rerender } = await render(undefined);
		expect(result.current.isDuplicationReady).toBe(false);

		for (let i = 0; i < 10; i++) {
			await rerender({ workspace: undefined });
			expect(result.current.isDuplicationReady).toBe(false);
		}
	});

	it("Will become ready when workspace is provided and build params are successfully fetched", async () => {
		const { result } = await render(M.MockWorkspace);
		expect(result.current.isDuplicationReady).toBe(false);
		await waitFor(() => expect(result.current.isDuplicationReady).toBe(true));
	});

	it("Is able to navigate the user to the workspace creation page", async () => {
		const { result, getLocationSnapshot } = await render(M.MockWorkspace);
		await performNavigation(result, getLocationSnapshot);
	});

	test("Navigating populates the URL search params with the workspace's build params", async () => {
		const mockBuildParams = [
			M.MockWorkspaceBuildParameter1,
			M.MockWorkspaceBuildParameter2,
			M.MockWorkspaceBuildParameter3,
			M.MockWorkspaceBuildParameter4,
			M.MockWorkspaceBuildParameter5,
		];

		const { result, getLocationSnapshot } = await render(M.MockWorkspace);
		await performNavigation(result, getLocationSnapshot);

		const { search } = getLocationSnapshot();
		for (const { name, value } of mockBuildParams) {
			const key = `param.${name}`;
			expect(search.get(key)).toEqual(value);
		}
	});

	test("Navigating appends other necessary metadata to the search params", async () => {
		const extraMetadataEntries: readonly [string, string][] = [
			["mode", "duplicate"],
			["name", `${M.MockWorkspace.name}-copy`],
			["version", M.MockWorkspace.template_active_version_id],
		];

		const { result, getLocationSnapshot } = await render(M.MockWorkspace);
		await performNavigation(result, getLocationSnapshot);

		const { search } = getLocationSnapshot();
		for (const [key, value] of extraMetadataEntries) {
			expect(search.get(key)).toBe(value);
		}
	});
});
