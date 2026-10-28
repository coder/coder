import type { Foundation } from "#/api/typesGenerated";

/** Prepare output rows without reading values unless metadata marks them public. */
export const getFoundationOutputs = (
	foundation: Pick<Foundation, "outputs" | "output_metadata">,
) => {
	const names = new Set([
		...Object.keys(foundation.output_metadata ?? {}),
		...Object.keys(foundation.outputs),
	]);
	return Array.from(names)
		.sort()
		.map((name) => {
			const metadata = foundation.output_metadata?.[name];
			return {
				name,
				type: JSON.stringify(metadata?.type) ?? "Unknown",
				sensitivity:
					metadata?.sensitive === false
						? "Public"
						: metadata?.sensitive === true
							? "Sensitive"
							: "Unknown",
				value:
					metadata?.sensitive === false
						? (JSON.stringify(foundation.outputs[name], null, 2) ??
							"Unavailable")
						: "********",
			};
		});
};
