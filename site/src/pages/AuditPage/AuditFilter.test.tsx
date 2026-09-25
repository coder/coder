import { act, renderHook, waitFor } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { useResourceTypeFilterMenu } from "./AuditFilter";

const createWrapper = (): FC<PropsWithChildren> => {
	const queryClient = createTestQueryClient();
	return ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
};

describe("useResourceTypeFilterMenu", () => {
	it("filters by experiment rules with a friendly label", async () => {
		const onChange = vi.fn();
		const { result } = renderHook(
			() => useResourceTypeFilterMenu({ value: undefined, onChange }),
			{ wrapper: createWrapper() },
		);

		await waitFor(() => expect(result.current.searchOptions).toBeDefined());
		const option = result.current.searchOptions?.find(
			(o) => o.value === "experiment_rule",
		);
		expect(option).toEqual({
			value: "experiment_rule",
			label: "Experiment Rule",
		});

		act(() => result.current.selectOption(option));
		expect(onChange).toHaveBeenCalledWith(option);
	});
});
