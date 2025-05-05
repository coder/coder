import type { TemplateVersionExternalAuth } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { Check, Redo } from "lucide-react";
import type { FC } from "react";

export interface ExternalAuthButtonProps {
	auth: TemplateVersionExternalAuth;
	displayRetry: boolean;
	isLoading: boolean;
	onStartPolling: () => void;
	error?: unknown;
}

export const ExternalAuthButton: FC<ExternalAuthButtonProps> = ({
	auth,
	displayRetry,
	isLoading,
	onStartPolling,
	error,
}) => {
	return (
		<div className="flex items-center gap-2 border border-border border-solid rounded-md p-3 justify-between">
			<span className="flex flex-row items-center gap-2">
				{auth.display_icon && (
					<ExternalImage
						className="w-6 h-6"
						src={auth.display_icon}
						alt={`${auth.display_name} Icon`}
					/>
				)}
				<p className="font-semibold m-0">{auth.display_name}</p>
				{!auth.optional && (
					<Badge size="sm" variant={error ? "destructive" : "warning"}>
						Required
					</Badge>
				)}
			</span>

			<span className="flex flex-row items-center gap-2">
				{auth.authenticated ? (
					<>
						<Check className="w-5 h-5 text-content-success" />
						<p className="text-sm font-semibold text-content-secondary m-0">
							Authenticated
						</p>
					</>
				) : (
					<Button
						variant="default"
						size="sm"
						disabled={isLoading || auth.authenticated}
						onClick={() => {
							window.open(
								auth.authenticate_url,
								"_blank",
								"width=900,height=600",
							);
							onStartPolling();
						}}
					>
						<Spinner loading={isLoading} />
						Login with {auth.display_name}
					</Button>
				)}

				{displayRetry && !auth.authenticated && (
					<TooltipProvider>
						<Tooltip delayDuration={100}>
							<TooltipTrigger asChild>
								<Button variant="outline" size="icon" onClick={onStartPolling}>
									<Redo />
									<span className="sr-only">Refresh external auth</span>
								</Button>
							</TooltipTrigger>
							<TooltipContent>
								Retry login with {auth.display_name}
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				)}
			</span>
		</div>
	);
};
