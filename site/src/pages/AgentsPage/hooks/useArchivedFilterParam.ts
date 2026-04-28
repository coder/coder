import { useSearchParamsKey } from "#/hooks/useSearchParamsKey";

type ArchivedFilter = "active" | "archived";

const toArchivedFilter = (value: string): ArchivedFilter =>
	value === "archived" ? "archived" : "active";

/**
 * Reads and writes the agents page's archived filter via the `?archived` URL
 * search param. Unknown or missing values fall back to `"active"`. Setting the
 * filter back to `"active"` removes the param from the URL so the default
 * state has a single canonical URL (`/agents`).
 */
export const useArchivedFilterParam = (): readonly [
	ArchivedFilter,
	(next: ArchivedFilter) => void,
] => {
	const param = useSearchParamsKey({
		key: "archived",
		defaultValue: "active",
	});
	const filter = toArchivedFilter(param.value);
	const setFilter = (next: ArchivedFilter) => {
		if (next === "active") {
			param.deleteValue();
			return;
		}
		param.setValue(next);
	};
	return [filter, setFilter] as const;
};
