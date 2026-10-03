import { act, renderHook } from "@testing-library/react";
import type { UpdateUserAppearanceSettingsRequest } from "#/api/typesGenerated";
import { useQueuedAppearanceSubmit } from "./AppearancePage";

const updateRequest = (
	overrides: Partial<UpdateUserAppearanceSettingsRequest> = {},
): UpdateUserAppearanceSettingsRequest => ({
	theme_preference: "dark",
	theme_mode: "single",
	theme_light: "light",
	theme_dark: "dark",
	terminal_font: "geist-mono",
	...overrides,
});

describe("useQueuedAppearanceSubmit", () => {
	it("submits one request at a time and keeps only the latest pending update", () => {
		const mutations: Array<{
			values: UpdateUserAppearanceSettingsRequest;
			onSettled: () => void;
		}> = [];
		const mutate = vi.fn(
			(
				values: UpdateUserAppearanceSettingsRequest,
				options: { onSettled: () => void },
			) => {
				mutations.push({ values, onSettled: options.onSettled });
			},
		);
		const { result } = renderHook(() => useQueuedAppearanceSubmit(mutate));

		const first = updateRequest({ terminal_font: "fira-code" });
		const overwritten = updateRequest({ theme_preference: "dark-tritan" });
		const latest = updateRequest({ theme_preference: "light" });

		act(() => {
			result.current(first);
			result.current(overwritten);
			result.current(latest);
		});

		expect(mutate).toHaveBeenCalledTimes(1);
		expect(mutations[0]?.values).toEqual(first);

		act(() => {
			mutations[0]?.onSettled();
		});

		expect(mutate).toHaveBeenCalledTimes(2);
		expect(mutations[1]?.values).toEqual(latest);

		act(() => {
			mutations[1]?.onSettled();
			result.current(overwritten);
		});

		expect(mutate).toHaveBeenCalledTimes(3);
		expect(mutations[2]?.values).toEqual(overwritten);
	});
});
