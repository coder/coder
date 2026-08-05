import { QueryClient } from "react-query";
import { describe, expect, it } from "vitest";
import type { Template } from "#/api/typesGenerated";
import { MockTemplate } from "#/testHelpers/entities";
import {
	getTemplatesQueryKey,
	templateByNameKey,
	templateExamples,
	updateTemplateListQueries,
	updateTemplateMeta,
} from "./templates";

describe("updateTemplateListQueries", () => {
	it("updates every cached template list and preserves other templates", async () => {
		const queryClient = new QueryClient();
		const defaultListKey = getTemplatesQueryKey();
		const emptyFilterListKey = getTemplatesQueryKey({ q: "" });
		const otherTemplate: Template = {
			...MockTemplate,
			id: "other-template",
			name: "other-template",
		};
		const updatedTemplate: Template = {
			...MockTemplate,
			agents_allowed: false,
		};
		queryClient.setQueryData(defaultListKey, [MockTemplate, otherTemplate]);
		queryClient.setQueryData(emptyFilterListKey, [MockTemplate, otherTemplate]);

		await updateTemplateListQueries(queryClient, updatedTemplate);

		expect(queryClient.getQueryData(defaultListKey)).toEqual([
			updatedTemplate,
			otherTemplate,
		]);
		expect(queryClient.getQueryData(emptyFilterListKey)).toEqual([
			updatedTemplate,
			otherTemplate,
		]);
		expect(queryClient.getQueryState(defaultListKey)?.isInvalidated).toBe(true);
		expect(queryClient.getQueryState(emptyFilterListKey)?.isInvalidated).toBe(
			true,
		);
	});

	it("does not update or invalidate non-list template queries", async () => {
		const queryClient = new QueryClient();
		const examplesKey = templateExamples().queryKey;
		const objectSegmentKey = ["templates", { view: "summary" }];
		queryClient.setQueryData(examplesKey, [{ id: "example" }]);
		queryClient.setQueryData(objectSegmentKey, { count: 1 });

		await updateTemplateListQueries(queryClient, {
			...MockTemplate,
			agents_allowed: false,
		});

		expect(queryClient.getQueryData(examplesKey)).toEqual([{ id: "example" }]);
		expect(queryClient.getQueryState(examplesKey)?.isInvalidated).toBe(false);
		expect(queryClient.getQueryData(objectSegmentKey)).toEqual({ count: 1 });
		expect(queryClient.getQueryState(objectSegmentKey)?.isInvalidated).toBe(
			false,
		);
	});
});

describe("updateTemplateMeta", () => {
	it("invalidates the detail query for the returned name after a rename", async () => {
		const queryClient = new QueryClient();
		const renamedTemplate: Template = {
			...MockTemplate,
			name: "renamed-template",
		};
		const oldNameKey = templateByNameKey(
			MockTemplate.organization_name,
			MockTemplate.name,
		);
		const newNameKey = templateByNameKey(
			renamedTemplate.organization_name,
			renamedTemplate.name,
		);
		queryClient.setQueryData(oldNameKey, MockTemplate);
		queryClient.setQueryData(newNameKey, renamedTemplate);

		await updateTemplateMeta(queryClient).onSuccess?.(
			renamedTemplate,
			{
				template: MockTemplate,
				data: { name: renamedTemplate.name },
			},
			undefined,
		);

		expect(queryClient.getQueryState(newNameKey)?.isInvalidated).toBe(true);
		expect(queryClient.getQueryState(oldNameKey)?.isInvalidated).toBe(false);
	});
});
