import { cn } from "cn";
import { Button } from "#/components/Button/Button";

/** Which owners' automations the list shows. */
export type AutomationOwnerView = "mine" | "all";

const options: readonly { value: AutomationOwnerView; label: string }[] = [
	{ value: "mine", label: "Mine" },
	{ value: "all", label: "All" },
];

type AutomationOwnerFilterProps = {
	value: AutomationOwnerView;
	onChange: (value: AutomationOwnerView) => void;
};

/** Switches the list between the viewer's automations and every owner's. */
export const AutomationOwnerFilter: React.FC<AutomationOwnerFilterProps> = ({
	value,
	onChange,
}) => (
	<div
		role="group"
		aria-label="Owner"
		className="flex items-center rounded-md border border-solid border-border p-0.5"
	>
		{options.map((option) => {
			const isActive = option.value === value;
			return (
				<Button
					key={option.value}
					type="button"
					size="sm"
					variant="subtle"
					aria-pressed={isActive}
					className={cn(
						"h-7 px-3",
						isActive && "bg-surface-tertiary text-content-primary",
					)}
					onClick={() => onChange(option.value)}
				>
					{option.label}
				</Button>
			);
		})}
	</div>
);
