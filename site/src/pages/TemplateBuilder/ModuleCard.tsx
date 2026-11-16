import { cn } from "cn";
import { BadgeCheckIcon } from "lucide-react";
import { useId } from "react";
import { Avatar } from "#/components/Avatar/Avatar";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { Link } from "#/components/Link/Link";

type ModuleCardProps = {
	name: string;
	description: string;
	iconUrl?: string;
	detailsUrl?: string;
	official?: boolean;
	selected?: boolean;
	onSelect?: () => void;
};

export const ModuleCard: React.FC<ModuleCardProps> = ({
	name,
	description,
	iconUrl,
	detailsUrl,
	official = true,
	selected = false,
	onSelect,
}) => {
	const checkboxId = useId();
	const descriptionId = useId();
	return (
		<div
			className={cn(
				"relative isolate flex flex-col pt-4 px-4 pb-6 rounded",
				"bg-surface-secondary border",
				selected && "border-border-pending",
			)}
		>
			<div className="flex items-start justify-between mb-3">
				<Avatar src={iconUrl} size="lg" variant="icon" />
				<Checkbox
					id={checkboxId}
					checked={selected}
					aria-describedby={descriptionId}
					onCheckedChange={onSelect}
					className="z-10 m-0"
				/>
			</div>

			<div className="flex flex-col gap-2">
				<h3 className="my-0 text-sm font-bold text-content-primary">
					{/* The label stretches over the card so clicking anywhere toggles it. */}
					<label
						htmlFor={checkboxId}
						className="cursor-pointer after:absolute after:inset-0 after:content-['']"
					>
						{name}
						{official && (
							<>
								{" "}
								<BadgeCheckIcon className="size-4 text-highlight-sky align-middle inline-block" />
								<span className="sr-only"> (official)</span>
							</>
						)}
					</label>
				</h3>
				<p
					id={descriptionId}
					className="my-0 text-xs font-normal text-content-secondary"
				>
					{description}
				</p>

				<Link href={detailsUrl} target="_blank" className="text-xs font-normal">
					View details<span className="sr-only"> for {name}</span>
				</Link>
			</div>
		</div>
	);
};
