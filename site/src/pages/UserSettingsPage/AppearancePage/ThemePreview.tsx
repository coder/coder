import type { CSSProperties, FC } from "react";
import { baseModeFor, type ConcreteThemeName } from "#/theme";
import { cn } from "#/utils/cn";

interface ThemePreviewProps {
	/**
	 * The concrete theme name applied as a CSS class on the outermost
	 * element so the preview reads that theme's CSS variables. The base
	 * mode class (`light` or `dark`) is applied alongside it so the
	 * preview self-contains all surface and content tokens regardless
	 * of the ambient theme on `<html>`.
	 */
	theme: ConcreteThemeName;
	/**
	 * `sm` matches the legacy 224px tile used by the single-theme grid;
	 * `lg` fills its container so the sync cards can be any width.
	 */
	size?: "sm" | "lg";
	label?: string;
	className?: string;
	style?: CSSProperties;
}

/**
 * Mini mockup of the Coder UI under a given theme. Used both inside
 * the single-theme grid tiles (sm) and inside the two sync-mode cards
 * (lg). The header bar's two accent swatches visibly change between
 * colorblind variants because they use `bg-git-added` / `bg-git-deleted`.
 */
export const ThemePreview: FC<ThemePreviewProps> = ({
	theme,
	size = "sm",
	label,
	className,
	style,
}) => {
	return (
		<div className={cn(baseModeFor(theme), theme)}>
			<div
				className={cn(
					"overflow-clip rounded-md border border-border border-solid bg-surface-primary text-content-primary select-none",
					size === "sm" ? "w-56" : "w-full",
					className,
				)}
				style={style}
			>
				<div className="bg-surface-primary text-content-primary">
					<div
						className={cn(
							"bg-surface-primary flex items-center justify-between border-0 border-b border-border border-solid",
							size === "sm" ? "px-2.5 py-1.5 mb-2" : "px-4 py-2.5 mb-3",
						)}
					>
						<div
							className={cn(
								"flex items-center",
								size === "sm" ? "gap-1.5" : "gap-2",
							)}
						>
							<div
								className={cn(
									"bg-content-primary rounded",
									size === "sm" ? "h-1.5 w-5" : "h-2 w-8",
								)}
							/>
							<div
								className={cn(
									"bg-content-secondary rounded",
									size === "sm" ? "h-1.5 w-5" : "h-2 w-8",
								)}
							/>
							<div
								className={cn(
									"bg-content-secondary rounded",
									size === "sm" ? "h-1.5 w-5" : "h-2 w-8",
								)}
							/>
						</div>
						<div
							className={cn(
								"flex items-center",
								size === "sm" ? "gap-1.5" : "gap-2",
							)}
						>
							{/* Two accent swatches mirror the diff additions and
							    deletions so the preview visibly changes between
							    colorblind variants. */}
							<div
								className={cn(
									"bg-git-added rounded",
									size === "sm" ? "h-1.5 w-3" : "h-2 w-4",
								)}
							/>
							<div
								className={cn(
									"bg-git-deleted rounded",
									size === "sm" ? "h-1.5 w-3" : "h-2 w-4",
								)}
							/>
						</div>
					</div>
					<div
						className={cn(
							"mx-auto",
							size === "sm" ? "w-32 pb-2" : "w-full max-w-md px-4 pb-6",
						)}
					>
						<div
							className={cn(
								"bg-content-primary rounded mb-1.5",
								size === "sm" ? "h-2 w-11" : "h-3 w-24",
							)}
						/>
						<div className="flex gap-2">
							<div
								className={cn(
									"rounded-md flex-1",
									size === "sm" ? "h-6" : "h-10",
									// Surface-git-added gives the filled progress bar the
									// same palette-aware fingerprint the accent swatches
									// use. Colorblind variants swap the hue accordingly.
									"bg-surface-git-added border border-solid border-border-default",
								)}
							>
								<div
									className={cn(
										"bg-git-added h-full rounded-md",
										size === "sm" ? "w-7" : "w-20",
									)}
								/>
							</div>
							<div
								className={cn(
									"rounded-md bg-surface-secondary",
									size === "sm" ? "w-10 h-6" : "w-16 h-10",
								)}
							/>
						</div>
					</div>
				</div>
				{label && (
					<div
						className={cn(
							"border-0 border-t border-border border-solid font-medium text-content-primary",
							size === "sm" ? "px-2.5 py-1.5 text-xs" : "px-4 py-2 text-sm",
						)}
					>
						{label}
					</div>
				)}
			</div>
		</div>
	);
};
