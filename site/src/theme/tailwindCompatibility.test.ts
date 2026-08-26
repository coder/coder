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

function dataAttributeCount(selector: string): number {
	return selector.split("[data-").length - 1;
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

	it("preserves Tailwind v3's default ring color and width", async () => {
		const cssPath = path.resolve(__dirname, "../index.css");
		const source = `${fs.readFileSync(cssPath, "utf8")}\n@source inline("ring ring-2");`;
		const result = await postcss([tailwindcss()]).process(source, {
			from: cssPath,
		});
		const root = postcss.parse(result.css);
		const ringShadows = new Map<string, string>();
		const ringSelectors = new Set([".ring", ".ring-2"]);

		root.walkRules((rule) => {
			if (!ringSelectors.has(rule.selector)) {
				return;
			}
			rule.walkDecls("--tw-ring-shadow", (declaration) => {
				ringShadows.set(rule.selector, declaration.value);
			});
		});

		expect(ringShadows.get(".ring")).toContain(
			"calc(3px + var(--tw-ring-offset-width))",
		);
		expect(ringShadows.get(".ring-2")).toContain(
			"var(--tw-ring-color, rgb(59 130 246 / 0.5))",
		);
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

	it("preserves directional popper animation transforms", async () => {
		const cssPath = path.resolve(__dirname, "../index.css");
		const source = fs.readFileSync(cssPath, "utf8");
		const result = await postcss([tailwindcss()]).process(source, {
			from: cssPath,
		});
		const root = postcss.parse(result.css);
		const directionalRules: Array<{ property: string; selector: string }> = [];
		const resetRules: Array<{ property: string; selector: string }> = [];
		const independentTranslations: string[] = [];
		const positionedTransforms = new Map<string, string>();

		root.walkRules((rule) => {
			const declarations = new Map<string, string>();
			rule.walkDecls((declaration) => {
				declarations.set(declaration.prop, declaration.value);
			});

			for (const property of [
				"--tw-enter-translate-x",
				"--tw-enter-translate-y",
			]) {
				const value = declarations.get(property);
				if (value === "initial") {
					resetRules.push({ property, selector: rule.selector });
				} else if (
					value !== undefined &&
					rule.selector.includes(String.raw`data-\[side\=`)
				) {
					directionalRules.push({ property, selector: rule.selector });
				}
			}

			if (!rule.selector.includes(String.raw`data-\[side\=`)) {
				return;
			}
			if (declarations.has("translate")) {
				independentTranslations.push(rule.selector);
			}
			const side = rule.selector.match(
				/\[data-side="(bottom|left|right|top)"\]/,
			)?.[1];
			const transform = declarations.get("transform");
			if (side !== undefined && transform?.startsWith("translate") === true) {
				positionedTransforms.set(side, transform);
			}
		});

		const stateOpenDirectionalRules = directionalRules.filter((directional) =>
			directional.selector.includes(String.raw`data-\[state\=open\]`),
		);
		expect(stateOpenDirectionalRules).toHaveLength(4);
		const stateOpenResets = resetRules.filter((reset) =>
			reset.selector.includes(String.raw`data-\[state\=open\]\:animate-in`),
		);
		expect(stateOpenResets.length).toBeGreaterThan(0);
		for (const directional of stateOpenDirectionalRules) {
			for (const reset of stateOpenResets) {
				if (reset.property === directional.property) {
					expect(dataAttributeCount(directional.selector)).toBeGreaterThan(
						dataAttributeCount(reset.selector),
					);
				}
			}
		}
		expect(independentTranslations).toEqual([]);
		expect(Object.fromEntries(positionedTransforms)).toEqual({
			bottom: "translateY(var(--spacing))",
			left: "translateX(calc(var(--spacing) * -1))",
			right: "translateX(var(--spacing))",
			top: "translateY(calc(var(--spacing) * -1))",
		});
	});
});
