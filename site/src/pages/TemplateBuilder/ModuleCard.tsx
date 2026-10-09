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
	const nameId = useId();
	return (
		// oxlint-disable-next-line jsx-a11y/click-events-have-key-events -- Clicking the card is a mouse shortcut; keyboard users toggle the checkbox.
		<div
			className={cn(
				"flex flex-col pt-4 px-4 pb-6 rounded",
				"bg-surface-secondary border border-solid",
				"cursor-pointer",
				selected ? "border-border-pending" : "border-border",
			)}
			onClick={(e) => {
				// The checkbox and the details link handle their own clicks.
				if (e.target instanceof Element && e.target.closest("a, button")) {
					return;
				}
				onSelect?.();
			}}
		>
			<div className="flex items-start justify-between mb-3">
				<Avatar src={iconUrl} size="lg" variant="icon" />
				<Checkbox
					checked={selected}
					onCheckedChange={() => onSelect?.()}
					aria-labelledby={nameId}
					className="m-0"
				/>
			</div>

			<div className="flex flex-col gap-2">
				<h3 id={nameId} className="my-0 text-sm font-bold text-content-primary">
					{name}
					{official && (
						<>
							{" "}
							<BadgeCheckIcon className="size-4 text-highlight-sky align-middle inline-block" />
						</>
					)}
				</h3>
				<p className="my-0 text-xs font-normal text-content-secondary">
					{description}
				</p>

				<Link href={detailsUrl} target="_blank" className="text-xs font-normal">
					View details
				</Link>
			</div>
		</div>
	);
};
