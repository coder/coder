import type { Interpolation, Theme } from "@emotion/react";
import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown";
import Button from "@mui/material/Button";
import { CodeExample } from "components/CodeExample/CodeExample";
import {
	HelpTooltipLink,
	HelpTooltipLinksGroup,
	HelpTooltipText,
} from "components/HelpTooltip/HelpTooltip";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { Stack } from "components/Stack/Stack";
import { type ClassName, useClassName } from "hooks/useClassName";
import type { FC } from "react";
import { docs } from "utils/docs";

export interface SSHButtonProps {
	workspaceName: string;
	agentName: string;
	sshPrefix?: string;
}

export const SSHButton: FC<SSHButtonProps> = ({
	workspaceName,
	agentName,
	sshPrefix,
}) => {
	const paper = useClassName(classNames.paper, []);

	return (
		<Popover>
			<PopoverTrigger>
				<Button
					size="small"
					variant="text"
					endIcon={<KeyboardArrowDown />}
					css={{ fontSize: 13, padding: "8px 12px" }}
				>
					Connect via SSH
				</Button>
			</PopoverTrigger>

			<PopoverContent horizontal="right" classes={{ paper }}>
				<HelpTooltipText>
					Run the following commands to connect with SSH:
				</HelpTooltipText>

				<Stack spacing={0.5} css={styles.codeExamples}>
					<div>
						<HelpTooltipText>
							<strong css={styles.codeExampleLabel}>
								Configure SSH hosts on machine:
							</strong>
						</HelpTooltipText>
						<CodeExample secret={false} code="coder config-ssh" />
					</div>

					<div>
						<HelpTooltipText>
							<strong css={styles.codeExampleLabel}>
								Connect to the agent:
							</strong>
						</HelpTooltipText>
						<CodeExample
							secret={false}
							code={`ssh ${sshPrefix}${workspaceName}.${agentName}`}
						/>
					</div>
				</Stack>

				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/install")}>
						Install Coder CLI
					</HelpTooltipLink>
					<HelpTooltipLink href={docs("/user-guides/workspace-access/vscode")}>
						Connect via VS Code Remote SSH
					</HelpTooltipLink>
					<HelpTooltipLink
						href={docs("/user-guides/workspace-access/jetbrains")}
					>
						Connect via JetBrains Gateway
					</HelpTooltipLink>
					<HelpTooltipLink href={docs("/user-guides/workspace-access#ssh")}>
						SSH configuration
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</PopoverContent>
		</Popover>
	);
};

const classNames = {
	paper: (css, theme) => css`
    padding: 16px 24px 24px;
    width: 304px;
    color: ${theme.palette.text.secondary};
    margin-top: 2px;
  `,
} satisfies Record<string, ClassName>;

const styles = {
	codeExamples: {
		marginTop: 12,
	},

	codeExampleLabel: {
		fontSize: 12,
	},
} satisfies Record<string, Interpolation<Theme>>;
