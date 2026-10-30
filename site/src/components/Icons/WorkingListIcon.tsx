import { isPixel } from "@coder/pixel-storybook/storyapi";

type WorkingListIconProps = React.ComponentProps<"svg"> & {
	animated: boolean;
};

/** lucide's `ListTreeIcon`, animated unless reduced motion or Pixel capture. */
export const WorkingListIcon: React.FC<WorkingListIconProps> = ({
	animated,
	...props
}) => {
	return (
		<svg
			xmlns="http://www.w3.org/2000/svg"
			width="24"
			height="24"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			strokeWidth="2"
			strokeLinecap="round"
			strokeLinejoin="round"
			aria-hidden="true"
			{...props}
		>
			<path d="M3 6v4c0 1.1.9 2 2 2h3" />
			<path d="M3 10v6c0 1.1.9 2 2 2h3" />
			{animated && !isPixel() ? (
				<>
					{/* Staggered one step (0.9s) apart in the 3.6s cycle. */}
					{[
						"[animation-delay:0s]",
						"[animation-delay:-0.9s]",
						"[animation-delay:-1.8s]",
						"[animation-delay:-2.7s]",
					].map((delayClass) => (
						<path
							key={delayClass}
							d="M13 6h8"
							className={`animate-working-line transform-fill origin-left motion-reduce:hidden ${delayClass}`}
						/>
					))}
					<path
						d="M21 6H8M21 12h-8M21 18h-8"
						className="hidden motion-reduce:inline"
					/>
				</>
			) : (
				<path d="M21 6H8M21 12h-8M21 18h-8" />
			)}
		</svg>
	);
};
