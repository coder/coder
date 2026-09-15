import { sanitizeDiagramSvg, wrapInFence } from "./MermaidBlock";

describe("sanitizeDiagramSvg", () => {
	it("keeps Mermaid's drawing primitives and styles", () => {
		const svg = [
			'<svg id="mermaid-1" viewBox="0 0 10 10" xmlns="http://www.w3.org/2000/svg">',
			"<style>#mermaid-1 .node rect { fill: #333; }</style>",
			'<defs><marker id="arrow"><path d="M0,0 L10,5"/></marker></defs>',
			'<g class="node"><rect width="4" height="4"/><text><tspan>Start</tspan></text></g>',
			"</svg>",
		].join("");

		const result = sanitizeDiagramSvg(svg);

		expect(result).toContain("<style>");
		expect(result).toContain("<marker");
		expect(result).toContain("<tspan>Start</tspan>");
	});

	it("strips elements that would fetch external resources or navigate", () => {
		const svg = [
			'<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">',
			'<image href="https://attacker.invalid/track.png" width="1" height="1"/>',
			'<use xlink:href="https://attacker.invalid/sprite.svg#icon"/>',
			'<a href="https://attacker.invalid"><text>click</text></a>',
			'<foreignObject><iframe src="https://attacker.invalid"></iframe><div>Label</div></foreignObject>',
			"<script>alert(1)</script>",
			"</svg>",
		].join("");

		const result = sanitizeDiagramSvg(svg);

		expect(result).not.toContain("attacker.invalid");
		expect(result).not.toContain("<image");
		expect(result).not.toContain("<use");
		expect(result).not.toContain("<a ");
		expect(result).not.toContain("<iframe");
		expect(result).not.toContain("<script");
		expect(result).not.toContain("<foreignObject");
	});
});

describe("wrapInFence", () => {
	it("uses a plain triple-backtick fence for ordinary source", () => {
		expect(wrapInFence("flowchart LR\n  A --> B")).toBe(
			"```mermaid\nflowchart LR\n  A --> B\n```",
		);
	});

	it("extends the fence past the longest backtick run in the source", () => {
		const source = 'flowchart LR\n  A["uses ```` in a label"]';
		expect(wrapInFence(source)).toBe(
			`\`\`\`\`\`mermaid\n${source}\n\`\`\`\`\``,
		);
	});
});
