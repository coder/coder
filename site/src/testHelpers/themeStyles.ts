import fs from "node:fs";
import path from "node:path";

/** Installs base theme declarations in jsdom, which cannot parse Tailwind's stylesheet. */
export function installThemeStyles(): () => void {
	const css = fs.readFileSync(path.resolve(__dirname, "../index.css"), "utf8");
	const palette = fs.readFileSync(
		path.resolve(__dirname, "../../node_modules/tailwindcss/theme.css"),
		"utf8",
	);
	const colors = new Map(
		Array.from(palette.matchAll(/(--color-[\w-]+): ([^;]+);/g), (match) => [
			match[1],
			match[2],
		]),
	);
	const style = document.createElement("style");
	style.textContent = ["dark", "light"]
		.map((mode) => {
			const block = css.match(new RegExp(`\\t\\.${mode} \\{([^}]+)\\}`));
			if (!block) throw new Error(`Missing ${mode} theme declarations`);
			return `.${mode} {${block[1]}}`.replace(
				/var\((--color-[\w-]+)\)/g,
				(_match, name: string) => {
					const color = colors.get(name);
					if (!color) throw new Error(`Missing Tailwind color ${name}`);
					return color;
				},
			);
		})
		.join("\n");
	document.head.append(style);
	return () => style.remove();
}
