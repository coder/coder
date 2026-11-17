import { type SVGProps, useId } from "react";
import { useTheme } from "#/theme/context";

/** Decorative preview of an empty search result. */
export const SearchIllustration: React.FC<SVGProps<SVGSVGElement>> = (
	props,
) => {
	const dark = useTheme().palette.mode === "dark";
	const clipId = useId();

	return (
		<svg
			width="360"
			height="220"
			viewBox="0 0 360 220"
			fill="none"
			xmlns="http://www.w3.org/2000/svg"
			aria-hidden="true"
			{...props}
		>
			<g clipPath={`url(#${clipId})`}>
				<image
					width="360"
					height="200"
					href={
						dark
							? "/supergraphic/empty-state-dark.png"
							: "/supergraphic/empty-state-light.png"
					}
				/>
				<path
					d="M26 52C26 37.6406 37.6406 26 52 26H308C322.359 26 334 37.6406 334 52V200H26V52Z"
					className="fill-surface-primary"
				/>
				<rect
					x="118"
					y="95"
					width="36"
					height="36"
					rx="6"
					className="fill-surface-quaternary"
				/>
				<rect
					x="162"
					y="95"
					width="36"
					height="36"
					rx="6"
					className="fill-surface-tertiary"
				/>
				<rect
					x="206"
					y="95"
					width="36"
					height="36"
					rx="6"
					className="fill-surface-secondary"
				/>
			</g>
			<rect
				x="0.5"
				y="0.5"
				width="359"
				height="199"
				rx="11.5"
				className="stroke-border-secondary"
			/>
			<path
				d="M270 209L265.388 203.433M262.314 199.721L250.016 184.874M259.753 167.4C259.753 179.771 249.735 189.8 237.377 189.8C225.018 189.8 215 179.771 215 167.4C215 155.029 225.018 145 237.377 145C249.735 145 259.753 155.029 259.753 167.4Z"
				strokeWidth="3.5"
				strokeLinejoin="round"
				className="stroke-content-primary"
			/>
			<defs>
				<clipPath id={clipId}>
					<rect width="360" height="200" rx="12" />
				</clipPath>
			</defs>
		</svg>
	);
};
