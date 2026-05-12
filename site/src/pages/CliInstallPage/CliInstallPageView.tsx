import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import { Welcome } from "#/components/Welcome/Welcome";
import { cn } from "#/utils/cn";

type CliInstallPageViewProps = {
	origin: string;
};

export const CliInstallPageView: FC<CliInstallPageViewProps> = ({ origin }) => {
	return (
		<div
			className={cn(
				"mx-auto h-screen w-[480px]",
				"flex flex-1 flex-col items-center justify-center",
			)}
		>
			<Welcome>Install the Coder CLI</Welcome>

			<p className="pb-2 text-center text-base leading-[1.4] text-content-secondary">
				Copy the command below and{" "}
				<strong className="block">paste it in your terminal.</strong>
			</p>

			<CodeExample
				className="max-w-full"
				code={`curl -fsSL ${origin}/install.sh | sh`}
				secret={false}
			/>

			<div className="pt-4">
				<RouterLink
					to="/workspaces"
					className="block py-4 text-center text-content-primary underline decoration-[hsla(0,0%,100%,0.7)] underline-offset-[3px] hover:no-underline"
				>
					Go to workspaces
				</RouterLink>
			</div>
			<div className="mt-6 text-xs text-content-secondary">
				&copy; {new Date().getFullYear()} Coder Technologies, Inc.
			</div>
		</div>
	);
};
