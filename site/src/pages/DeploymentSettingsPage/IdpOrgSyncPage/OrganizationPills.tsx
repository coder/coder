import { useTheme } from "@emotion/react";
import { Pill } from "components/Pill/Pill";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { type FC, useState } from "react";
import { cn } from "utils/cn";
import { isUUID } from "utils/uuid";

interface OrganizationPillsProps {
	organizations: readonly string[];
}

export const OrganizationPills: FC<OrganizationPillsProps> = ({
	organizations,
}) => {
	const orgs = organizations.map((org) => ({
		name: org,
		isUUID: isUUID(org),
	}));

	return (
		<div className="flex flex-row gap-2">
			{orgs.length > 0 ? (
				<Pill
					className={cn("border-none w-fit", {
						"bg-surface-destructive": orgs[0].isUUID,
						"bg-surface-secondary": !orgs[0].isUUID,
					})}
				>
					{orgs[0].name}
				</Pill>
			) : (
				<p>None</p>
			)}

			{orgs.length > 1 && <OverflowPillList organizations={orgs.slice(1)} />}
		</div>
	);
};

interface OverflowPillProps {
	organizations: { name: string; isUUID: boolean }[];
}

const OverflowPillList: FC<OverflowPillProps> = ({ organizations }) => {
	const theme = useTheme();
	const [open, setOpen] = useState(false);

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger
				onMouseEnter={() => setOpen(true)}
				onMouseLeave={() => setOpen(false)}
			>
				<Pill
					className="min-h-4 min-w-6 bg-surface-secondary border-none px-3 py-1"
					data-testid="overflow-pill"
				>
					+{organizations.length}
				</Pill>
			</PopoverTrigger>

			<PopoverContent
				align="center"
				side="top"
				css={{
					display: "flex",
					flexFlow: "column wrap",
					columnGap: 8,
					rowGap: 12,
					padding: "12px 16px",
					alignContent: "space-around",
					minWidth: "auto",
					backgroundColor: theme.palette.background.default,
					marginBottom: "4px",
				}}
			>
				<ul className="list-none my-0 pl-0">
					{organizations.map((organization) => (
						<li key={organization.name} className="mb-2 last:mb-0">
							<Pill
								className={cn("border-none w-fit", {
									"bg-surface-destructive": organization.isUUID,
									"bg-surface-secondary": !organization.isUUID,
								})}
							>
								{organization.name}
							</Pill>
						</li>
					))}
				</ul>
			</PopoverContent>
		</Popover>
	);
};
