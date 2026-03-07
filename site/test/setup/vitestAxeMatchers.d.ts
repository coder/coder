import type { AxeMatchers } from "vitest-axe/matchers";

declare module "@vitest/expect" {
	interface Assertion<_T = unknown> extends AxeMatchers {}
	interface AsymmetricMatchersContaining extends AxeMatchers {}
	interface JestAssertion<_T = unknown> extends AxeMatchers {}
}

declare global {
	namespace jest {
		interface Matchers<_R = void, _T = unknown> extends AxeMatchers {}
	}
}
