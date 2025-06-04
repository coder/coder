import {
	ProvisionerKeyNameBuiltIn,
	ProvisionerKeyNamePSK,
	ProvisionerKeyNameUserAuth,
} from "api/typesGenerated";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { InfoIcon } from "lucide-react";
import type { FC, ReactNode } from "react";

type KeyType = "builtin" | "userAuth" | "psk" | "key";

function getKeyType(name: string) {
	switch (name) {
		case ProvisionerKeyNameBuiltIn:
			return "builtin";
		case ProvisionerKeyNameUserAuth:
			return "userAuth";
		case ProvisionerKeyNamePSK:
			return "psk";
		default:
			return "key";
	}
}

const infoByType: Record<KeyType, ReactNode> = {
	builtin: (
		<>
			These provisioners are running as part of a coderd instance. Built-in
			provisioners are only available for the default organization.{" "}
		</>
	),
	userAuth: (
		<>
			These provisioners are connected by users using the <code>coder</code>{" "}
			CLI, and are authorized by the users credentials. They can be tagged to
			only run provisioner jobs for that user. User-authenticated provisioners
			are only available for the default organization.
		</>
	),
	psk: (
		<>
			These provisioners all use pre-shared key authentication. PSK provisioners
			are only available for the default organization.
		</>
	),
	key: null,
};

type ProvisionerKeyProps = {
	name: string;
};

export const ProvisionerKey: FC<ProvisionerKeyProps> = ({ name }) => {
	const type = getKeyType(name);
	const info = infoByType[type];

	return (
		<span className="flex items-center gap-1 whitespace-nowrap text-xs font-medium text-content-secondary">
			{name}
			{info && (
				<TooltipProvider>
					<Tooltip delayDuration={0}>
						<TooltipTrigger asChild>
							<span className="flex items-center">
								<span className="sr-only">More info</span>
								<InfoIcon
									tabIndex={0}
									className="cursor-pointer size-icon-xs p-0.5"
								/>
							</span>
						</TooltipTrigger>
						<TooltipContent className="max-w-xs">
							{infoByType[type]}
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			)}
		</span>
	);
};
