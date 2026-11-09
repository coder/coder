import type { Theme } from ".";

/** Reads theme CSS colors, optionally converting them to hex for Monaco. */
export function readThemeColors(
	mode: Theme["mode"],
	properties: readonly string[],
	format: "css" | "hex" = "css",
): Record<string, string> {
	const probe = document.createElement("div");
	probe.className = mode;
	probe.hidden = true;
	document.body.append(probe);
	try {
		const styles = getComputedStyle(probe);
		let context: CanvasRenderingContext2D | null = null;
		if (format === "hex") {
			const canvas = document.createElement("canvas");
			canvas.width = canvas.height = 1;
			context = canvas.getContext("2d", { willReadFrequently: true });
			if (!context) {
				throw new Error("A 2D canvas is required to resolve theme colors");
			}
		}
		return Object.fromEntries(
			properties.map((property) => {
				const color = styles.getPropertyValue(property).trim();
				if (!context || /^#[0-9a-f]{6}$/i.test(color)) {
					return [property, color];
				}
				// Canvas converts CSS colors, including Tailwind's OKLCH palette, to sRGB bytes.
				context.clearRect(0, 0, 1, 1);
				context.fillStyle = color;
				context.fillRect(0, 0, 1, 1);
				const pixel = context.getImageData(0, 0, 1, 1).data;
				const hex = Array.from(pixel.slice(0, 3), (channel) =>
					channel.toString(16).padStart(2, "0"),
				).join("");
				return [property, `#${hex}`];
			}),
		);
	} finally {
		probe.remove();
	}
}
