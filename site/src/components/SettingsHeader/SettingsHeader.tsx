import { Button } from "components/Button/Button";
import { SquareArrowOutUpRightIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { twMerge } from "tailwind-merge";

type HeaderHierarchy = "primary" | "secondary";
type HeaderLevel = `h${1 | 2 | 3 | 4 | 5 | 6}`;

type HeaderProps = Readonly<{
	title: ReactNode;
	description?: ReactNode;
	titleVisualHierarchy?: HeaderHierarchy;
	titleHeaderLevel?: HeaderLevel;
	docsHref?: string;
	tooltip?: ReactNode;
}>;

export const SettingsHeader: FC<HeaderProps> = ({
	title,
	description,
	docsHref,
	tooltip,
	titleHeaderLevel = "h1",
	titleVisualHierarchy = "primary",
}) => {
	const Header = titleHeaderLevel;
	return (
		<hgroup className="flex flex-row justify-between align-baseline">
			{/*
			 * The text-sm class only adjusts the size of the description, but
			 * we need to apply it here so that it combines with the max-w-prose
			 * class and makes sure we have a predictable max width for the
			 * entire section of text.
			 */}
			<div className="text-sm max-w-prose pb-6">
				<div className="flex flex-row gap-2 align-middle">
					<Header
						className={twMerge(
							"m-0 text-3xl font-bold flex align-baseline leading-relaxed gap-2",
							titleVisualHierarchy === "secondary" && "text-2xl font-medium",
						)}
					>
						{title}
					</Header>
					{tooltip}
				</div>

				{description && (
					<p className="m-0 text-content-secondary leading-relaxed">
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
		</hgroup>
	);
};
