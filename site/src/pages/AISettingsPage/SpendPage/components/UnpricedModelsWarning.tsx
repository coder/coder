import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import { Link, type To } from "react-router";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/icons/AIBridgeModelIcon";

type UnpricedModelsWarningProps = {
	/** Accessible name of the warning trigger. */
	label: string;
	/**
	 * Models without pricing, or undefined when they cannot be determined,
	 * such as when the viewer cannot read model prices.
	 */
	models: readonly string[] | undefined;
	/** Where to set model pricing; only admins who can set it get a link. */
	setPricingHref: To | undefined;
	/** Visible text beside the icon. */
	children?: string;
	align?: "start" | "end";
};

/**
 * A warning icon whose hover card explains that spend excludes usage of
 * models without pricing, and lists those models.
 */
export const UnpricedModelsWarning: FC<UnpricedModelsWarningProps> = ({
	label,
	models,
	setPricingHref,
	children,
	align = "start",
}) => {
	const knownModels = models !== undefined && models.length > 0 ? models : [];
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					aria-label={children ? undefined : label}
					className="inline-flex cursor-default items-center gap-2 border-0 bg-transparent p-0 text-sm text-content-secondary [&_svg]:size-4 [&_svg]:shrink-0 [&_svg]:text-content-warning"
				>
					<TriangleAlertIcon />
					{children}
				</button>
			</TooltipTrigger>
			<TooltipContent
				side="bottom"
				align={align}
				className="flex w-72 flex-col gap-3 p-4 text-left font-normal"
			>
				<div className="flex flex-col gap-1">
					<p className="m-0 text-sm font-medium text-content-primary">
						Model pricing missing
					</p>
					<p className="m-0 text-sm leading-normal text-content-secondary">
						{knownModels.length === 0
							? "Some usage during this period was recorded without model pricing."
							: knownModels.length === 1
								? "1 model used during this period doesn't have pricing configured."
								: `${knownModels.length} models used during this period don't have pricing configured.`}{" "}
						Their usage isn't included, so actual spend may be higher than
						what's shown.
					</p>
				</div>
				{knownModels.length > 0 && (
					<ul
						aria-label="Models without pricing"
						className="m-0 flex max-h-32 list-none flex-col gap-2 overflow-y-auto rounded-md border border-solid border-border p-3"
					>
						{knownModels.map((model) => (
							<li
								key={model}
								className="flex min-w-0 items-center gap-2 text-sm text-content-secondary"
							>
								<AIBridgeModelIcon
									model={model}
									className="size-icon-sm shrink-0"
								/>
								<span className="truncate" title={model}>
									{model}
								</span>
							</li>
						))}
					</ul>
				)}
				{setPricingHref !== undefined && (
					<Link
						to={setPricingHref}
						className="text-sm text-content-link no-underline hover:underline"
					>
						Set pricing for these models
					</Link>
				)}
			</TooltipContent>
		</Tooltip>
	);
};
