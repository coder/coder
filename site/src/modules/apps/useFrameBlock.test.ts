import { act, renderHook } from "@testing-library/react";
import { type FrameBlock, useFrameBlock } from "./useFrameBlock";

type Violation = Partial<
	Pick<
		SecurityPolicyViolationEvent,
		"effectiveDirective" | "disposition" | "blockedURI" | "originalPolicy"
	>
>;

const appSrc =
	"https://code-server--main--workspace--alice.apps.example.com:8443/";
const appOrigin =
	"https://code-server--main--workspace--alice.apps.example.com:8443";
const coderPolicy = "frame-src 'self'; report-uri /api/v2/csp/reports";

// jsdom has no SecurityPolicyViolationEvent constructor.
function reportViolation(violation: Violation) {
	const event = Object.assign(new Event("securitypolicyviolation"), {
		effectiveDirective: "frame-src",
		disposition: "enforce",
		blockedURI: appOrigin,
		originalPolicy: coderPolicy,
		...violation,
	});
	act(() => {
		document.dispatchEvent(event);
	});
}

describe("useFrameBlock", () => {
	const cases: {
		name: string;
		src: string;
		violations: Violation[];
		expected: FrameBlock | undefined;
	}[] = [
		{
			name: "reports a block by Coder's policy",
			src: appSrc,
			violations: [{}],
			expected: { src: appSrc, origin: appOrigin, byCoderPolicy: true },
		},
		{
			name: "reports a block by a reverse proxy's policy",
			src: appSrc,
			violations: [{ originalPolicy: "frame-src 'self'" }],
			expected: { src: appSrc, origin: appOrigin, byCoderPolicy: false },
		},
		{
			name: "keeps a reverse proxy's block when Coder's policy also blocks",
			src: appSrc,
			violations: [{ originalPolicy: "frame-src 'self'" }, {}],
			expected: { src: appSrc, origin: appOrigin, byCoderPolicy: false },
		},
		{
			name: "matches a full blocked URL of a relative src",
			src: "/@alice/workspace.main/apps/code-server/",
			violations: [
				{
					blockedURI: `${location.origin}/@alice/workspace.main/apps/code-server/`,
				},
			],
			expected: {
				src: "/@alice/workspace.main/apps/code-server/",
				origin: location.origin,
				byCoderPolicy: true,
			},
		},
		{
			name: "ignores another origin",
			src: appSrc,
			violations: [{ blockedURI: "https://other.apps.example.com:8443" }],
			expected: undefined,
		},
		{
			name: "ignores a report-only policy",
			src: appSrc,
			violations: [{ disposition: "report" }],
			expected: undefined,
		},
		{
			name: "ignores other directives",
			src: appSrc,
			violations: [{ effectiveDirective: "img-src" }],
			expected: undefined,
		},
	];

	it.each(cases)("$name", ({ src, violations, expected }) => {
		const { result } = renderHook(() => useFrameBlock(src));
		for (const violation of violations) {
			reportViolation(violation);
		}
		expect(result.current).toEqual(expected);
	});

	it("clears the block when src changes", () => {
		const { result, rerender } = renderHook(({ src }) => useFrameBlock(src), {
			initialProps: { src: appSrc },
		});
		reportViolation({});
		expect(result.current).toBeDefined();

		rerender({
			src: "https://jupyter--main--workspace--alice.apps.example.com:8443/",
		});
		expect(result.current).toBeUndefined();
	});
});
