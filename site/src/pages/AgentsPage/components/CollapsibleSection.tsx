import { ChevronDownIcon } from "lucide-react";
import {
	type FC,
	type KeyboardEvent,
	type ReactNode,
	useId,
	useState,
} from "react";
import { cn } from "#/utils/cn";

interface CollapsibleSectionProps {
	title: string;
	description?: string;
	badge?: ReactNode;
	action?: ReactNode;
	defaultOpen?: boolean;
	children: ReactNode;
}

export const CollapsibleSection: FC<CollapsibleSectionProps> = ({
	title,
	description,
	badge,
	action,
	defaultOpen,
	children,
}) => {
	const contentId = useId();
	const [open, setOpen] = useState(defaultOpen ?? true);

	const toggle = () => setOpen((prev) => !prev);

	const handleKeyDown = (e: KeyboardEvent) => {
		if (e.key === "Enter" || e.key === " ") {
			e.preventDefault();
			toggle();
		}
	};

	return (
		<div className="rounded-lg border border-solid border-border-default">
			{/* Header — clickable toggle area */}
			<div
				className={cn(
					"flex cursor-pointer justify-between gap-4 px-6 py-5",
					description ? "items-start" : "items-center",
				)}
				role="button"
				tabIndex={0}
				aria-expanded={open}
				aria-controls={contentId}
				onClick={toggle}
				onKeyDown={handleKeyDown}
			>
				<div className="min-w-0 flex-1">
					<div className="flex items-center gap-2">
						<h2 className="m-0 text-lg font-semibold text-content-primary">
							{title}
						</h2>
						{badge && <span className="[&>*]:ml-0">{badge}</span>}
					</div>
					{description && (
						<p className="m-0 mt-1 text-sm text-content-secondary">
							{description}
						</p>
					)}
				</div>
				<div className="flex shrink-0 items-center gap-2">
					{action && (
						<div
							className="flex items-center"
							onClick={(e) => e.stopPropagation()}
							onKeyDown={(e) => e.stopPropagation()}
						>
							{action}
						</div>
					)}
					<div className="flex h-5 items-center">
						<ChevronDownIcon
							className={cn(
								"h-4 w-4 text-content-secondary transition-transform duration-200",
								open && "rotate-180",
							)}
						/>
					</div>
				</div>
			</div>
			{/* Divider + content */}
			{open && (
				<>
					<hr className="m-0 border-0 border-t border-solid border-border-default" />
					<div id={contentId} className="px-6 pb-5 pt-4">
						{children}
					</div>
				</>
			)}
		</div>
	);
};
