import { type SVGProps, useId } from "react";
import { useTheme } from "#/theme/context";

/** Decorative preview of content requiring access. */
export const NoAccessIllustration: React.FC<SVGProps<SVGSVGElement>> = (
	props,
) => {
	const dark = useTheme().palette.mode === "dark";
	const maskId = useId();
	const blurId = useId();
	const clipId = useId();

	return (
		<svg
			width="360"
			height="220"
			viewBox="0 0 360 220"
			fill="none"
			aria-hidden="true"
			xmlns="http://www.w3.org/2000/svg"
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
				<mask
					id={maskId}
					className="mask-type-alpha"
					maskUnits="userSpaceOnUse"
					x="26"
					y="26"
					width="308"
					height="174"
				>
					<rect x="26" y="26" width="308" height="174" fill="white" />
				</mask>
				<g mask={`url(#${maskId})`}>
					<g filter={`url(#${blurId})`}>
						<path
							d="M56 62C56 58.6863 58.6863 56 62 56H92C95.3137 56 98 58.6863 98 62V82C98 85.3137 95.3137 88 92 88H62C58.6863 88 56 85.3137 56 82V62Z"
							className="fill-surface-tertiary"
						/>
						<path
							d="M118 62C118 58.6863 120.686 56 124 56H228C231.314 56 234 58.6863 234 62V82C234 85.3137 231.314 88 228 88H124C120.686 88 118 85.3137 118 82V62Z"
							className="fill-surface-tertiary"
						/>
						<path
							d="M56 116.75C56 113.436 58.6863 110.75 62 110.75H230C233.314 110.75 236 113.436 236 116.75V136.75C236 140.064 233.314 142.75 230 142.75H62C58.6863 142.75 56 140.064 56 136.75V116.75Z"
							className="fill-surface-tertiary"
						/>
						<path
							d="M256 116.75C256 113.436 258.686 110.75 262 110.75H366C369.314 110.75 372 113.436 372 116.75V136.75C372 140.064 369.314 142.75 366 142.75H262C258.686 142.75 256 140.064 256 136.75V116.75Z"
							className="fill-surface-tertiary"
						/>
						<path
							d="M56 174.25C56 170.936 58.6863 168.25 62 168.25H116C119.314 168.25 122 170.936 122 174.25V194.25C122 197.564 119.314 200.25 116 200.25H62C58.6863 200.25 56 197.564 56 194.25V174.25Z"
							className="fill-surface-tertiary"
						/>
						<path
							d="M142 174.25C142 170.936 144.686 168.25 148 168.25H376C379.314 168.25 382 170.936 382 174.25V194.25C382 197.564 379.314 200.25 376 200.25H148C144.686 200.25 142 197.564 142 194.25V174.25Z"
							className="fill-surface-tertiary"
						/>
					</g>
				</g>
			</g>
			<rect
				x="0.5"
				y="0.5"
				width="359"
				height="199"
				rx="11.5"
				className="stroke-border-secondary"
			/>
			<rect
				x="157"
				y="174"
				width="47"
				height="35"
				className="fill-content-invert"
			/>
			<path
				d="M165 175H158V208H203V175H196M165 175V150.75M165 175H196M166.75 149H194.25M196 175V150.75"
				strokeWidth="3.5"
				className="stroke-content-primary"
			/>
			<path
				d="M180.5 188.25V194.25"
				strokeWidth="3.5"
				className="stroke-content-primary"
			/>
			<defs>
				<filter
					id={blurId}
					x="44"
					y="44"
					width="350"
					height="168.25"
					filterUnits="userSpaceOnUse"
					colorInterpolationFilters="sRGB"
				>
					<feGaussianBlur stdDeviation="6" />
				</filter>
				<clipPath id={clipId}>
					<rect width="360" height="200" rx="12" />
				</clipPath>
			</defs>
		</svg>
	);
};
