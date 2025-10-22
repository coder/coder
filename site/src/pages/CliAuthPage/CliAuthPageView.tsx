import { Button } from "components/Button/Button";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Spinner } from "components/Spinner/Spinner";
import { Welcome } from "components/Welcome/Welcome";
import { useClipboard } from "hooks";
import { CheckIcon, CopyIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";

interface CliAuthPageViewProps {
	sessionToken?: string;
}

export const CliAuthPageView: FC<CliAuthPageViewProps> = ({ sessionToken }) => {
	const clipboardState = useClipboard();
	return (
		<SignInLayout>
			<Welcome>Session token</Welcome>

			<p className="m-0 text-center text-sm text-content-secondary leading-normal">
				Copy the session token below and{" "}
				<strong className="block">paste it in your terminal.</strong>
			</p>

			<div className="flex flex-col items-center gap-1 w-full mt-4">
				<Button
					className="w-full"
					size="lg"
					disabled={!sessionToken}
					onClick={() => {
						if (sessionToken) {
							clipboardState.copyToClipboard(sessionToken);
						}
					}}
				>
					{clipboardState.showCopiedSuccess ? (
						<CheckIcon />
					) : (
						<Spinner loading={!sessionToken}>
							<CopyIcon />
						</Spinner>
					)}
					{clipboardState.showCopiedSuccess
						? "Session token copied!"
						: "Copy session token"}
				</Button>

				<Button className="w-full" variant="subtle" asChild>
					<RouterLink to="/workspaces">Go to workspaces</RouterLink>
				</Button>
			</div>
		</SignInLayout>
	);
};
