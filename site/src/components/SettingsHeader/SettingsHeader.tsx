import { Button } from "components/Button/Button";
import { SquareArrowOutUpRightIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { twMerge } from "tailwind-merge";

type HeaderHierarchy = "primary" | "secondary";

type HeaderProps = Readonly<{
	title: ReactNode;
	description?: ReactNode;
	hierarchy?: HeaderHierarchy;
	docsHref?: string;
	tooltip?: ReactNode;
}>;

export const SettingsHeader: FC<HeaderProps> = ({
	title,
	description,
	docsHref,
	tooltip,
	hierarchy = "primary",
}) => {
	return (
		<div className="flex flex-row justify-between align-baseline">
			<div className="max-w-prose pb-6">
				<div className="flex flex-row gap-2 align-middle">
					<h1
						className={twMerge(
							"m-0 text-3xl font-bold flex align-baseline leading-relaxed gap-2",
							hierarchy === "secondary" && "text-2xl font-medium",
						)}
					>
						{title}
					</h1>
					{tooltip}
				</div>

				{description && (
					<p className="m-0 text-sm text-content-secondary leading-relaxed">
						{description}
					</p>
				)}
			</div>

			{docsHref && (
				<Button asChild variant="outline">
					<a href={docsHref} target="_blank" rel="noreferrer">
						<SquareArrowOutUpRightIcon />
						Read the docs
						<span className="sr-only"> (link opens in new tab)</span>
					</a>
				</Button>
			)}
		</div>
	);
};
