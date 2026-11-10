import { cn } from "cn";
import { BadgeCheckIcon } from "lucide-react";
import { useId } from "react";
import { Avatar } from "#/components/Avatar/Avatar";
import { Link } from "#/components/Link/Link";
import { RadioGroupItem } from "#/components/RadioGroup/RadioGroup";

type TemplateCardProps = {
	value: string;
	name: string;
	description: string;
	iconUrl?: string;
	detailsUrl: string;
	official?: boolean;
	selected?: boolean;
	onSelect?: () => void;
};

export const TemplateCard: React.FC<TemplateCardProps> = ({
	value,
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
		// oxlint-disable-next-line jsx-a11y/click-events-have-key-events -- Clicking the card is a mouse shortcut; keyboard users select the radio.
		<div
			className={cn(
				"flex flex-col pt-4 px-4 pb-6 rounded",
				"bg-surface-secondary border border-solid",
				"cursor-pointer",
				selected ? "border-border-pending" : "border-border",
			)}
			onClick={() => onSelect?.()}
		>
			<div className="flex items-start justify-between mb-3">
				<Avatar src={iconUrl} size="lg" variant="icon" />
				{/* No change handler: clicks, Space, and arrow keys bubble up to the card as clicks. */}
				<RadioGroupItem
					value={value}
					aria-labelledby={nameId}
					className="mt-0.5 shrink-0 focus-visible:ring-offset-surface-secondary"
				/>
			</div>

			<div className="flex flex-col gap-2">
				<h3 id={nameId} className="my-0 text-sm font-bold text-content-primary">
					{name}
					{official && (
						<BadgeCheckIcon className="relative bottom-0.5 inline size-4 ml-1 align-middle text-highlight-sky" />
					)}
				</h3>
				<div className="flex flex-col gap-2">
					<p className="my-0 text-xs font-normal text-content-secondary">
						{description}
					</p>

					<Link
						href={detailsUrl}
						target="_blank"
						className="text-xs font-normal"
					>
						View details
					</Link>
				</div>
			</div>
		</div>
	);
};
