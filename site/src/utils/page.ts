export const pageTitle = (...crumbs: string[]): string => {
	return [...crumbs, "Coder"].join(" - ");
};
