import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

// Use filesystem paths to avoid Vite's browser asset URL transform.
const moduleDir = path.dirname(fileURLToPath(import.meta.url));
const css = fs.readFileSync(path.resolve(moduleDir, "../index.css"), "utf8");
const palette = fs.readFileSync(
	path.resolve(moduleDir, "../../node_modules/tailwindcss/theme.css"),
	"utf8",
);
const colors = new Map(
	Array.from(palette.matchAll(/(--color-[\w-]+): ([^;]+);/g), (match) => [
		match[1],
		match[2],
	]),
);

/** Installs base theme declarations in jsdom, which cannot parse Tailwind's stylesheet. */
export function installThemeStyles(): () => void {
	const style = document.createElement("style");
	style.textContent = ["dark", "light"]
		.map((mode) => {
			const block = css.match(
				new RegExp(`^\\s*\\.${mode}\\s*\\{([^}]+)\\}`, "m"),
			);
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
