export const pageTitle = (
	...crumbs: Array<string | boolean | undefined | null>
): string => {
	return [...crumbs, "Coder"].filter(Boolean).join(" - ");
};
