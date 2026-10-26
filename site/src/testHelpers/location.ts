/**
 * Stubs `location` so assigning `location.href` calls the returned spy
 * instead of navigating, which jsdom can't do for custom protocols. Undo it
 * with `vi.unstubAllGlobals()`.
 */
export const stubLocationHref = () => {
	const setHref = vi.fn<(url: string) => void>();
	const { origin, href } = location;
	vi.stubGlobal("location", {
		origin,
		get href() {
			return href;
		},
		set href(url: string) {
			setHref(url);
		},
	});
	return setHref;
};
