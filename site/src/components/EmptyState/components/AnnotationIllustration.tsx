import { type SVGProps, useId } from "react";
import { useTheme } from "#/theme/context";

/** Decorative preview of a template annotation. */
export const AnnotationIllustration: React.FC<SVGProps<SVGSVGElement>> = (
	props,
) => {
	const dark = useTheme().palette.mode === "dark";
	const clipId = useId();
	const maskId = useId();

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
				<mask
					id={maskId}
					style={{
						maskType: "alpha",
					}}
					maskUnits="userSpaceOnUse"
					x="0"
					y="0"
					width="360"
					height="200"
				>
					<rect
						width="360"
						height="200"
						rx="10"
						className="fill-surface-primary"
					/>
				</mask>
				<g mask={`url(#${maskId})`}>
					<image
						width="360"
						height="200"
						href={
							dark
								? "/supergraphic/empty-state-dark.png"
								: "/supergraphic/empty-state-light.png"
						}
					/>
				</g>
				<path
					d="M26 52C26 37.6406 37.6406 26 52 26H360V200H26V52Z"
					className="fill-surface-primary"
				/>
				<path
					d="M45.75 55C45.75 49.8914 49.8914 45.75 55 45.75H359V48.25H55C51.2721 48.25 48.25 51.2721 48.25 55H45.75ZM359 199H47H359ZM45.75 199V55C45.75 49.8914 49.8914 45.75 55 45.75V48.25C51.2721 48.25 48.25 51.2721 48.25 55V199H45.75ZM359 47V199V47Z"
					className="fill-content-success"
				/>
				<rect
					x="91"
					y="129"
					width="184"
					height="50"
					rx="6"
					className="fill-surface-tertiary"
				/>
				<rect
					x="289"
					y="129"
					width="184"
					height="50"
					rx="6"
					className="fill-surface-tertiary"
				/>
				<rect
					x="91"
					y="90"
					width="74"
					height="26"
					rx="6"
					className="fill-surface-tertiary"
				/>
				<rect
					x="182.5"
					y="94.5"
					width="17"
					height="17"
					rx="8.5"
					className="fill-content-success"
				/>
				<rect
					x="182.5"
					y="94.5"
					width="17"
					height="17"
					rx="8.5"
					strokeWidth="7"
					className="stroke-surface-green"
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
				d="M212.383 151.267L207.963 155.67M186.651 176.886L182.231 181.289M186.038 165.165L180 163.554M195.321 154.014L193.704 148M200.376 165.902L238 179.025L221.203 190.432L210.912 208L197.96 168.429C197.837 168.076 197.813 167.694 197.89 167.327C197.968 166.961 198.144 166.625 198.398 166.359C198.652 166.094 198.973 165.91 199.323 165.829C199.674 165.748 200.039 165.773 200.376 165.902Z"
				strokeWidth="3.5"
				strokeLinecap="square"
				className="fill-content-invert stroke-content-primary"
			/>
			<defs>
				<clipPath id={clipId}>
					<rect
						width="360"
						height="200"
						rx="12"
						className="fill-surface-primary"
					/>
				</clipPath>
			</defs>
		</svg>
	);
};
