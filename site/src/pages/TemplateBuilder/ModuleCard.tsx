import { CheckIcon } from "lucide-react";
import { useId } from "react";
import { Link } from "#/components/Link/Link";
import { cn } from "#/utils/cn";

type ModuleCardProps = {
	name: string;
	description: string;
	iconUrl?: string;
	detailsUrl?: string;
	selected?: boolean;
	onSelect?: () => void;
};

export const ModuleCard: React.FC<ModuleCardProps> = ({
	name,
	description,
	iconUrl,
	detailsUrl,
	selected = false,
	onSelect,
}) => {
	const nameId = useId();
	return (
		<div
			role="checkbox"
			aria-checked={selected}
			aria-labelledby={nameId}
			tabIndex={0}
			className={cn(
				"flex flex-col pt-4 px-4 pb-6 rounded",
				"bg-surface-secondary border border-solid",
				"cursor-pointer",
				"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-border-primary",
				selected ? "border-border-pending" : "border-border",
			)}
			onClick={() => onSelect?.()}
			onKeyDown={(e) => {
				if (e.key === "Enter" || e.key === " ") {
					e.preventDefault();
					onSelect?.();
				}
			}}
		>
			<div className="flex items-start justify-between mb-3">
				<div className="flex items-center justify-center p-1 rounded-md size-10 shrink-0 bg-surface-secondary border border-solid border-border">
					{iconUrl ? (
						<img src={iconUrl} alt="" className="size-7 object-contain" />
					) : (
						<div className="size-7 rounded bg-surface-primary" />
					)}
				</div>
				<div
					aria-hidden="true"
					className={cn(
						"flex items-center justify-center size-4 rounded-xs mt-0.5 shrink-0",
						"border border-solid border-border-secondary",
						selected ? "bg-content-primary" : "bg-surface-secondary",
					)}
				>
					{selected && (
						<CheckIcon className="size-3 absolute text-content-invert" />
					)}
				</div>
			</div>

			<div>
				<h3 id={nameId} className="text-md font-semibold text-content-primary">
					{name}
				</h3>
				<p className="text-sm font-normal text-content-secondary">
					{description}
				</p>

				<div>
					<Link
						href={detailsUrl}
						target="_blank"
						className="text-sm font-normal"
					>
						View details
					</Link>
				</div>
			</div>
		</div>
	);
};
