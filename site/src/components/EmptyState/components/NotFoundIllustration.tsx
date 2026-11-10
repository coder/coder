import { type SVGProps, useId } from "react";
import { useTheme } from "#/theme/context";

/** Decorative preview of a page that could not be found. */
export const NotFoundIllustration: React.FC<SVGProps<SVGSVGElement>> = (
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
				<circle cx="52" cy="52" r="6" className="fill-surface-quaternary" />
				<circle cx="72" cy="52" r="6" className="fill-surface-quaternary" />
				<circle cx="92" cy="52" r="6" className="fill-surface-quaternary" />
				<path
					d="M146.468 156.965V147.003H124V141.957L146.081 111.035H152.021V141.569H157.573V147.003H152.021V156.965H146.468ZM129.682 141.569H146.468V118.798L129.682 141.569Z"
					className="fill-content-primary"
				/>
				<path
					d="M167.878 150.367L185.956 117.375L191.122 117.633L173.044 150.625L167.878 150.367ZM179.5 158C176.186 158 173.302 157.03 170.848 155.089C168.438 153.148 166.587 150.388 165.296 146.809C164.005 143.229 163.359 138.981 163.359 134.065C163.359 129.105 164.005 124.836 165.296 121.256C166.587 117.677 168.438 114.916 170.848 112.976C173.302 110.992 176.186 110 179.5 110C182.857 110 185.741 110.992 188.152 112.976C190.562 114.916 192.413 117.677 193.704 121.256C194.995 124.836 195.641 129.105 195.641 134.065C195.641 138.981 194.995 143.229 193.704 146.809C192.413 150.388 190.562 153.148 188.152 155.089C185.741 157.03 182.857 158 179.5 158ZM179.5 152.566C181.652 152.566 183.481 151.811 184.988 150.302C186.537 148.792 187.721 146.658 188.539 143.898C189.4 141.094 189.83 137.817 189.83 134.065C189.83 130.226 189.4 126.927 188.539 124.167C187.721 121.364 186.537 119.208 184.988 117.698C183.481 116.189 181.652 115.434 179.5 115.434C177.391 115.434 175.562 116.189 174.012 117.698C172.463 119.208 171.257 121.364 170.396 124.167C169.579 126.927 169.17 130.226 169.17 134.065C169.17 137.817 169.579 141.094 170.396 143.898C171.257 146.658 172.463 148.792 174.012 150.302C175.562 151.811 177.391 152.566 179.5 152.566Z"
					className="fill-content-primary"
				/>
				<path
					d="M223.895 156.965V147.003H201.427V141.957L223.508 111.035H229.447V141.569H235V147.003H229.447V156.965H223.895ZM207.108 141.569H223.895V118.798L207.108 141.569Z"
					className="fill-content-primary"
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
			<defs>
				<clipPath id={clipId}>
					<rect width="360" height="200" rx="12" />
				</clipPath>
			</defs>
		</svg>
	);
};
