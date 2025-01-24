export const pageTitle = (
	...crumbs: Array<string | false | null | undefined>
): string => {
	return [...crumbs, "Coder"].filter(Boolean).join(" - ");
};
