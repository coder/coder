export const normalizeLocationSearch = (search: string): string =>
	search === "" || search.startsWith("?") ? search : `?${search}`;
