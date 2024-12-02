import { useTheme } from "@emotion/react";
import { Pill } from "components/Pill/Pill";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import type { FC } from "react";
import { cn } from "utils/cn";

interface OrganizationPillsProps {
	organizations: readonly string[];
}

// used to check if the organization is a UUID
const UUID =
	/^[0-9a-f]{8}-[0-9a-f]{4}-[0-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

export const OrganizationPills: FC<OrganizationPillsProps> = ({
	organizations,
}) => {
	const orgs = organizations.map((org) => {
		return { name: org, isUUID: UUID.test(org) };
	});

	return (
		<div className="flex flex-row gap-2">
			{orgs.length > 0 ? (
				<Pill
					className={cn("border-none w-fit", {
						"bg-surface-error": orgs[0].isUUID,
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

	return (
		<Popover mode="hover">
			<PopoverTrigger>
				<Pill
					className="min-h-4 min-w-6 bg-surface-secondary border-none px-3 py-1"
					data-testid="overflow-pill"
				>
					+{organizations.length}
				</Pill>
			</PopoverTrigger>

			<PopoverContent
				disableRestoreFocus
				disableScrollLock
				css={{
					".MuiPaper-root": {
						display: "flex",
						flexFlow: "column wrap",
						columnGap: 8,
						rowGap: 12,
						padding: "12px 16px",
						alignContent: "space-around",
						minWidth: "auto",
						backgroundColor: theme.palette.background.default,
					},
				}}
				anchorOrigin={{
					vertical: -4,
					horizontal: "center",
				}}
				transformOrigin={{
					vertical: "bottom",
					horizontal: "center",
				}}
			>
				<ul className="list-none my-0 pl-0">
					{organizations.map((organization) => (
						<li key={organization.name} className="mb-2 last:mb-0">
							<Pill
								className={cn("border-none w-fit", {
									"bg-surface-error": organization.isUUID,
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
