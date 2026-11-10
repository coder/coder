/** Reads theme CSS colors, optionally converting them to hex for Monaco. */
export function readThemeColors(
	mode: "dark" | "light",
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
			canvas.width = 1;
			canvas.height = 1;
			context = canvas.getContext("2d", { willReadFrequently: true });
			if (!context) {
				throw new Error("A 2D canvas is required to resolve theme colors");
			}
		}
		return Object.fromEntries(
			properties.map((property) => {
				const color = styles.getPropertyValue(property).trim();
				if (!color) {
					throw new Error(`Missing theme color ${property} for ${mode} mode`);
				}
				if (!context) {
					return [property, color];
				}
				if (/^#(?:[0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/i.test(color)) {
					const hex = color.slice(1);
					// Monaco token colors require six or eight digits, not CSS shorthand.
					return [
						property,
						hex.length <= 4
							? `#${Array.from(hex, (digit) => digit.repeat(2)).join("")}`
							: color,
					];
				}
				// Invalid assignments are ignored; two sentinels avoid rejecting valid black.
				context.fillStyle = "#000000";
				context.fillStyle = color;
				if (context.fillStyle === "#000000") {
					context.fillStyle = "#ffffff";
					context.fillStyle = color;
					if (context.fillStyle === "#ffffff") {
						throw new Error(
							`Invalid theme color ${property} for ${mode} mode: ${color}`,
						);
					}
				}
				// Canvas converts CSS colors, including Tailwind's OKLCH palette, to sRGB bytes.
				context.clearRect(0, 0, 1, 1);
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
