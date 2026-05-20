import { ShieldIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

type SettingsNavItemProps = {
	icon: FC<{ className?: string }>;
	label: string;
	active: boolean;
	adminOnly?: boolean;
	disabled?: boolean;
	trailingIcon?: FC<{ className?: string }>;
} & (
	| { to: string; replace?: boolean; state?: unknown; onClick?: () => void }
	| { to?: never; replace?: never; state?: never; onClick: () => void }
);

const navItemClassName = (active: boolean, disabled: boolean | undefined) =>
	cn(
		"flex w-full items-center gap-2.5 rounded-md border-0 px-2.5 py-2 text-left text-sm cursor-pointer transition-colors no-underline",
		active
			? "bg-surface-quaternary/25 text-content-primary font-medium"
			: "bg-transparent text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
		disabled && "opacity-50 pointer-events-none",
	);

const NavItemContent: FC<{
	icon: FC<{ className?: string }>;
	label: string;
	adminOnly?: boolean;
	trailingIcon?: FC<{ className?: string }>;
}> = ({ icon: Icon, label, adminOnly, trailingIcon: TrailingIcon }) => (
	<>
		<Icon className="h-4 w-4 shrink-0" />
		<span className="min-w-0 flex-1">{label}</span>
		{(adminOnly || TrailingIcon) && (
			<span className="ml-auto flex items-center gap-2">
				{adminOnly && (
					<Tooltip>
						<TooltipTrigger asChild>
							<span className="inline-flex">
								<ShieldIcon className="h-3 w-3 shrink-0 opacity-50" />
							</span>
						</TooltipTrigger>
						<TooltipContent side="right">Admin only</TooltipContent>
					</Tooltip>
				)}
				{TrailingIcon && <TrailingIcon className="h-4 w-4 shrink-0" />}
			</span>
		)}
	</>
);

export const SettingsNavItem: FC<SettingsNavItemProps> = ({
	icon,
	label,
	active,
	adminOnly,
	disabled,
	trailingIcon,
	...rest
}) => {
	if (rest.to != null) {
		return (
			<Link
				to={rest.to}
				replace={rest.replace}
				state={rest.state}
				onClick={rest.onClick}
				className={navItemClassName(active, disabled)}
				aria-current={active ? "page" : undefined}
				tabIndex={disabled ? -1 : undefined}
			>
				<NavItemContent
					icon={icon}
					label={label}
					adminOnly={adminOnly}
					trailingIcon={trailingIcon}
				/>
			</Link>
		);
	}

	return (
		<button
			type="button"
			onClick={rest.onClick}
			disabled={disabled}
			className={navItemClassName(active, disabled)}
			aria-current={active ? "page" : undefined}
		>
			<NavItemContent
				icon={icon}
				label={label}
				adminOnly={adminOnly}
				trailingIcon={trailingIcon}
			/>
		</button>
	);
};
