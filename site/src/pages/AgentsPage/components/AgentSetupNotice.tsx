import { CheckIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";

const agentSetupLinkClassName =
	"text-content-link transition-colors hover:text-content-link/80";

interface AgentSetupNoticeProps {
	providerCount: number;
	modelCount: number;
}

export const AgentSetupNotice: FC<AgentSetupNoticeProps> = ({
	providerCount,
	modelCount,
}) => {
	const hasProvider = providerCount > 0;
	const hasModel = modelCount > 0;

	if (hasProvider && hasModel) {
		return null;
	}

	return (
		<Dialog open>
			<DialogContent
				className="w-fit max-w-[calc(100vw-2rem)] gap-8"
				onEscapeKeyDown={(event) => {
					event.preventDefault();
				}}
				onPointerDownOutside={(event) => {
					event.preventDefault();
				}}
			>
				<DialogHeader className="space-y-5 text-left sm:text-left">
					<DialogTitle className="text-xl">Welcome to Coder Agents</DialogTitle>
					<DialogDescription className="text-base">
						Complete 2 quick steps to get started.
					</DialogDescription>
				</DialogHeader>

				<div className="flex flex-col gap-3 text-base text-content-secondary">
					<AgentSetupStep
						isComplete={hasProvider}
						stepNumber={1}
						label="Connect a chat provider"
						linkTo="/agents/settings/providers"
						linkText="Go to Providers"
					/>
					<AgentSetupStep
						isComplete={hasModel}
						stepNumber={2}
						label="Add a chat model"
						linkTo="/agents/settings/models"
						linkText="Go to Models"
					/>
				</div>
			</DialogContent>
		</Dialog>
	);
};

interface AgentSetupStepProps {
	isComplete: boolean;
	stepNumber: number;
	label: string;
	linkTo: string;
	linkText: string;
}

const AgentSetupStep: FC<AgentSetupStepProps> = ({
	isComplete,
	stepNumber,
	label,
	linkTo,
	linkText,
}) => {
	return (
		<div className="flex flex-wrap items-center gap-x-3 gap-y-1">
			<span className="flex w-7 shrink-0 justify-end text-content-secondary">
				{isComplete ? (
					<CheckIcon
						aria-label="Complete"
						className="h-5 w-5 text-content-success"
					/>
				) : (
					`${stepNumber}.`
				)}
			</span>
			<span className="text-content-secondary">{label}</span>
			<Link to={linkTo} className={agentSetupLinkClassName}>
				{linkText}
			</Link>
		</div>
	);
};
