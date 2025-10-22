import { expect, type Page } from "@playwright/test";

type PollingOptions = { timeout?: number; intervals?: number[] };

export const expectUrl = expect.extend({
	/**
	 * toHavePathName is an alternative to `toHaveURL` that won't fail if the URL
	 * contains query parameters.
	 */
	async toHavePathName(page: Page, expected: string, options?: PollingOptions) {
		let actual: string = new URL(page.url()).pathname;
		let pass: boolean;
		try {
			await expect
				.poll(() => {
					actual = new URL(page.url()).pathname;
					return actual;
				}, options)
				.toBe(expected);
			pass = true;
		} catch {
			pass = false;
		}

		return {
			name: "toHavePathName",
			pass,
			actual,
			expected,
			message: () =>
				`The page does not have the expected URL pathname.\nExpected: ${
					this.isNot ? "not" : ""
				}${this.utils.printExpected(
					expected,
				)}\nActual: ${this.utils.printReceived(actual)}`,
		};
	},

	/**
	 * toHavePathNameEndingWith allows checking the end of the URL (ie. to make
	 * sure we redirected to a specific page) without caring about the entire URL,
	 * which might depend on things like whether or not organizations or other
	 * features are enabled.
	 */
	async toHavePathNameEndingWith(
		page: Page,
		expected: string,
		options?: PollingOptions,
	) {
		let actual: string = new URL(page.url()).pathname;
		let pass: boolean;
		try {
			await expect
				.poll(() => {
					actual = new URL(page.url()).pathname;
					return actual.endsWith(expected);
				}, options)
				.toBe(true);
			pass = true;
		} catch {
			pass = false;
		}

		return {
			name: "toHavePathNameEndingWith",
			pass,
			actual,
			expected,
			message: () =>
				`The page does not have the expected URL pathname.\nExpected a url ${
					this.isNot ? "not " : ""
				}ending with: ${this.utils.printExpected(
					expected,
				)}\nActual: ${this.utils.printReceived(actual)}`,
		};
	},
});
