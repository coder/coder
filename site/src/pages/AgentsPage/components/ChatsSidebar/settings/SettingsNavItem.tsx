import { cn } from "cn";
import { ShieldIcon } from "lucide-react";
import { Link } from "react-router";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type SettingsNavItemProps = {
	icon: React.FC<{ className?: string }>;
	label: string;
	active: boolean;
	adminOnly?: boolean;
	ariaLabel?: string;
	className?: string;
	disabled?: boolean;
	trailing?: React.ReactNode;
	trailingIcon?: React.FC<{ className?: string }>;
} & (
	| {
			to: React.ComponentProps<typeof Link>["to"];
			replace?: boolean;
			state?: unknown;
			onClick?: () => void;
	  }
	| { to?: never; replace?: never; state?: never; onClick: () => void }
);

const navItemClassName = (
	active: boolean,
	disabled: boolean | undefined,
	className: string | undefined,
) =>
	cn(
		"flex w-full items-center gap-2.5 rounded-md border-0 px-2.5 py-1.5 text-left text-sm cursor-pointer transition-colors no-underline",
		active
			? "bg-surface-quaternary/25 text-content-primary font-medium"
			: "bg-transparent text-content-secondary hover:bg-surface-tertiary/50 hover:text-content-primary",
		disabled && "opacity-50 pointer-events-none",
		className,
	);

const NavItemContent: React.FC<{
	icon: React.FC<{ className?: string }>;
	label: string;
	adminOnly?: boolean;
	trailing?: React.ReactNode;
	trailingIcon?: React.FC<{ className?: string }>;
}> = ({
	icon: Icon,
	label,
	adminOnly,
	trailing,
	trailingIcon: TrailingIcon,
}) => (
	<>
		<Icon className="size-4 shrink-0" />
		<span className="min-w-0 flex-1">{label}</span>
		{(adminOnly || trailing || TrailingIcon) && (
			<span className="ml-auto flex items-center gap-2">
				{adminOnly && (
					<Tooltip>
						<TooltipTrigger asChild>
							<span className="inline-flex">
								<ShieldIcon className="size-3 shrink-0 opacity-50" />
							</span>
						</TooltipTrigger>
						<TooltipContent side="right">Admin only</TooltipContent>
					</Tooltip>
				)}
				{TrailingIcon && <TrailingIcon className="size-4 shrink-0" />}
				{trailing}
			</span>
		)}
	</>
);

export const SettingsNavItem: React.FC<SettingsNavItemProps> = ({
	icon,
	label,
	active,
	adminOnly,
	ariaLabel,
	className,
	disabled,
	trailing,
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
				className={navItemClassName(active, disabled, className)}
				aria-current={active ? "page" : undefined}
				aria-label={ariaLabel}
				tabIndex={disabled ? -1 : undefined}
			>
				<NavItemContent
					icon={icon}
					label={label}
					adminOnly={adminOnly}
					trailing={trailing}
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
			className={navItemClassName(active, disabled, className)}
			aria-current={active ? "page" : undefined}
			aria-label={ariaLabel}
		>
			<NavItemContent
				icon={icon}
				label={label}
				adminOnly={adminOnly}
				trailing={trailing}
				trailingIcon={trailingIcon}
			/>
		</button>
	);
};
