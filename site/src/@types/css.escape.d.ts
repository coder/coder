// Polyfill for the unit tests only; see test/setup/polyfills.ts.
declare module "css.escape" {
	const cssEscape: (value: string) => string;
	export default cssEscape;
}
