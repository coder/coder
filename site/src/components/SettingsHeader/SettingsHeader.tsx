import { useTheme } from "@emotion/react";
import { Button } from "components/Button/Button";
import { Stack } from "components/Stack/Stack";
import { SquareArrowOutUpRightIcon } from "lucide-react";
import type { FC, ReactNode } from "react";

interface HeaderProps {
	title: ReactNode;
	description?: ReactNode;
	secondary?: boolean;
	docsHref?: string;
	tooltip?: ReactNode;
}

export const SettingsHeader: FC<HeaderProps> = ({
	title,
	description,
	docsHref,
	secondary,
	tooltip,
}) => {
	const theme = useTheme();

	return (
		<Stack alignItems="baseline" direction="row" justifyContent="space-between">
			<div css={{ maxWidth: 420, marginBottom: 24 }}>
				<Stack direction="row" spacing={1} alignItems="center">
					<h1
						css={[
							{
								fontSize: 32,
								fontWeight: 700,
								display: "flex",
								alignItems: "baseline",
								lineHeight: "initial",
								margin: 0,
								marginBottom: 4,
								gap: 8,
							},
							secondary && {
								fontSize: 24,
								fontWeight: 500,
							},
						]}
					>
						{title}
					</h1>
					{tooltip}
				</Stack>

				{description && (
					<span
						css={{
							fontSize: 14,
							color: theme.palette.text.secondary,
							lineHeight: "160%",
						}}
					>
						{description}
					</span>
				)}
			</div>

			{docsHref && (
				<Button asChild variant="outline">
					<a href={docsHref} target="_blank" rel="noreferrer">
						<SquareArrowOutUpRightIcon />
						Read the docs
					</a>
				</Button>
			)}
		</Stack>
	);
};
