import fs from "node:fs";
import path from "node:path";
import tailwindcss from "@tailwindcss/postcss";
import postcss, { type AtRule, type Node, type Rule } from "postcss";

const BUILT_IN_SIBLING_SELECTOR = /:where\((\.[^{]*?) > :not\(:last-child\)\)/g;

interface SiblingRule {
	media: string;
	order: number;
	properties: ReadonlyMap<string, string>;
	selectorPrefix: string;
}

function isAtRule(node: Node): node is AtRule {
	return node.type === "atrule";
}

function mediaContext(rule: Rule): string {
	const media: string[] = [];
	let parent: Node | undefined = rule.parent;
	while (parent !== undefined) {
		if (isAtRule(parent) && parent.name === "media") {
			media.unshift(parent.params);
		}
		parent = parent.parent;
	}
	return media.join(" | ");
}

function expectedProperties(properties: ReadonlyMap<string, string>): string[] {
	const propertyNames = [...properties.keys()];
	const nonCustomProperties = propertyNames.filter(
		(property) => !property.startsWith("--"),
	);
	return nonCustomProperties.length > 0 ? nonCustomProperties : propertyNames;
}

function hasSiblingSelector(css: string, selectorPrefix: string): boolean {
	return css.includes(`${selectorPrefix} > :not([hidden]) ~ :not([hidden])`);
}

describe("Tailwind CSS compatibility", () => {
	it("resets every emitted v4 sibling selector after its declaration", async () => {
		const cssPath = path.resolve(__dirname, "../index.css");
		const source = fs.readFileSync(cssPath, "utf8");
		const result = await postcss([tailwindcss()]).process(source, {
			from: cssPath,
		});
		const root = postcss.parse(result.css);
		const siblingRules: SiblingRule[] = [];
		let order = 0;

		root.walkRules((rule) => {
			order += 1;
			for (const match of rule.selector.matchAll(BUILT_IN_SIBLING_SELECTOR)) {
				const declarations = new Map<string, string>();
				rule.walkDecls((declaration) => {
					declarations.set(declaration.prop, declaration.value);
				});
				siblingRules.push({
					media: mediaContext(rule),
					order,
					properties: declarations,
					selectorPrefix: match[1],
				});
			}
		});

		const emittedRules = siblingRules.filter((rule) =>
			expectedProperties(rule.properties).some(
				(property) => rule.properties.get(property) !== "revert-layer",
			),
		);
		expect(emittedRules.length).toBeGreaterThan(0);

		for (const emittedRule of emittedRules) {
			const expected = expectedProperties(emittedRule.properties);
			expect(
				siblingRules.some(
					(candidate) =>
						candidate.selectorPrefix === emittedRule.selectorPrefix &&
						candidate.media === emittedRule.media &&
						candidate.order > emittedRule.order &&
						expected.every(
							(property) =>
								candidate.properties.get(property) === "revert-layer",
						),
				),
			).toBe(true);
			expect(hasSiblingSelector(result.css, emittedRule.selectorPrefix)).toBe(
				true,
			);
		}
	});

	it("keeps hover variants outside Tailwind v4's pointer media query", async () => {
		const cssPath = path.resolve(__dirname, "../index.css");
		const source = fs.readFileSync(cssPath, "utf8");
		const result = await postcss([tailwindcss()]).process(source, {
			from: cssPath,
		});
		const root = postcss.parse(result.css);
		const gatedHoverRules: string[] = [];
		let hoverRuleCount = 0;

		root.walkRules((rule) => {
			if (!rule.selector.includes(String.raw`hover\:`)) {
				return;
			}
			hoverRuleCount += 1;
			if (mediaContext(rule).includes("(hover: hover)")) {
				gatedHoverRules.push(rule.selector);
			}
		});

		expect(hoverRuleCount).toBeGreaterThan(0);
		expect(gatedHoverRules).toEqual([]);
	});
});
