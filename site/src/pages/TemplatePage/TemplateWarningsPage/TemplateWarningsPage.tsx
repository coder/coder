import { useTheme } from "@emotion/react";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Pill } from "components/Pill/Pill";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import {
	CircleAlertIcon,
	InfoIcon,
	RotateCcwIcon,
	TriangleAlertIcon,
	XIcon,
} from "lucide-react";
import { useTemplateLayoutContext } from "pages/TemplatePage/TemplateLayout";
import { type FC, useMemo, useState } from "react";
import { pageTitle } from "utils/page";

interface Warning {
	id: string;
	severity: "error" | "warning" | "info";
	title: string;
	message: string;
	code?: string;
	dismissed?: boolean;
}

// Placeholder for backend data - the user will implement the actual API call
const useTemplateWarnings = (templateVersionId: string): Warning[] => {
	// TODO: Replace this with actual API call once backend is implemented
	// Example: const { data } = useQuery(['templateWarnings', templateVersionId], () => API.getTemplateVersionWarnings(templateVersionId));

	// Mock data for UI demonstration
	return [
		{
			id: "1",
			severity: "error",
			title: "Terraform module 'coder-server' is not pinned to a version",
			message:
				"The 'coder-server' module should be pinned to a specific version to ensure stability and reproducibility. " +
				"Please update the module source to include a version constraint.",
			code: "ERR_UNPINNED_MODULE_VERSION",
		},
		{
			id: "2",
			severity: "error",
			title: "Missing terraform lock file",
			message:
				"The terraform lock file (terraform.lock.hcl) is missing. " +
				"Please generate the lock file to ensure consistent provider versions.",
			code: "ERR_MISSING_LOCK_FILE",
		},
		{
			id: "3",
			severity: "warning",
			title: "Unused coder parameter 'region' detected",
			message: "The parameter 'region' is defined but not used in the template. ",
			code: "WRN_UNUSED_PARAMETER",
		},
	];
};

const TemplateWarningsPage: FC = () => {
	const { template, activeVersion } = useTemplateLayoutContext();
	const theme = useTheme();
	const warnings = useTemplateWarnings(activeVersion.id);
	const [dismissedWarnings, setDismissedWarnings] = useState<Set<string>>(
		new Set(),
	);

	// Sort warnings: non-dismissed first, dismissed at the end
	const sortedWarnings = useMemo(() => {
		return [...warnings].sort((a, b) => {
			const aDismissed = dismissedWarnings.has(a.id) || a.dismissed;
			const bDismissed = dismissedWarnings.has(b.id) || b.dismissed;

			if (aDismissed === bDismissed) return 0;
			return aDismissed ? 1 : -1;
		});
	}, [warnings, dismissedWarnings]);

	const handleToggleDismiss = (warningId: string) => {
		setDismissedWarnings((prev) => {
			const next = new Set(prev);
			if (next.has(warningId)) {
				next.delete(warningId);
			} else {
				next.add(warningId);
			}
			return next;
		});
	};

	const isWarningDismissed = (warning: Warning) => {
		return dismissedWarnings.has(warning.id) || warning.dismissed;
	};

	const getSeverityIcon = (severity: string, dismissed: boolean) => {
		const iconSize = 16;
		const opacity = dismissed ? 0.4 : 1;

		switch (severity) {
			case "error":
				return (
					<CircleAlertIcon
						size={iconSize}
						css={{ color: theme.palette.error.main, opacity }}
					/>
				);
			case "warning":
				return (
					<TriangleAlertIcon
						size={iconSize}
						css={{ color: theme.palette.warning.main, opacity }}
					/>
				);
			case "info":
			default:
				return (
					<InfoIcon
						size={iconSize}
						css={{ color: theme.palette.info.main, opacity }}
					/>
				);
		}
	};

	const getSeverityPillType = (
		severity: string,
	): "error" | "warning" | "info" => {
		switch (severity) {
			case "error":
				return "error";
			case "warning":
				return "warning";
			case "info":
			default:
				return "info";
		}
	};

	return (
		<>
			<title>{pageTitle(template.name, "Warnings")}</title>

			<div
				css={{
					background: theme.palette.background.paper,
					border: `1px solid ${theme.palette.divider}`,
					borderRadius: 8,
					overflow: "hidden",
				}}
			>
				{/* Header */}
				<div
					css={{
						padding: "16px 24px",
						borderBottom: `1px solid ${theme.palette.divider}`,
					}}
				>
					<div
						css={{
							color: theme.palette.text.secondary,
							fontWeight: 600,
							fontSize: 14,
						}}
					>
						Warnings & Errors
					</div>
					<div
						css={{
							color: theme.palette.text.secondary,
							fontSize: 12,
							marginTop: 4,
						}}
					>
						Showing diagnostics for the active template version:{" "}
						<span css={{ fontWeight: 600 }}>{activeVersion.name}</span>
					</div>
				</div>

				{/* Content */}
				{sortedWarnings.length === 0 ? (
					<div css={{ padding: "64px 24px" }}>
						<EmptyState
							message="No warnings or errors"
							description="This template version looks good!"
						/>
					</div>
				) : (
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead css={{ width: 40 }} />
								<TableHead css={{ width: 120 }}>Severity</TableHead>
								<TableHead>Issue</TableHead>
								<TableHead css={{ width: 100 }}>Code</TableHead>
								<TableHead css={{ width: 40 }} />
							</TableRow>
						</TableHeader>
						<TableBody>
							{sortedWarnings.map((warning) => {
								const isDismissed = isWarningDismissed(warning);

								return (
									<TableRow
										key={warning.id}
										css={{
											opacity: isDismissed ? 0.5 : 1,
											transition: "opacity 0.2s ease",
										}}
									>
										<TableCell>
											<div css={{ display: "flex", alignItems: "center" }}>
												{getSeverityIcon(
													warning.severity,
													isDismissed || false,
												)}
											</div>
										</TableCell>
										<TableCell>
											{!isDismissed && (
												<Pill type={getSeverityPillType(warning.severity)}>
													{warning.severity.charAt(0).toUpperCase() +
														warning.severity.slice(1)}
												</Pill>
											)}
										</TableCell>
										<TableCell>
											{isDismissed ? (
												<div
													css={{
														fontSize: 13,
														color: theme.palette.text.secondary,
														fontStyle: "italic",
													}}
												>
													{warning.title}
												</div>
											) : (
												<div>
													<div
														css={{
															fontWeight: 600,
															fontSize: 14,
															color: theme.palette.text.primary,
															marginBottom: 4,
														}}
													>
														{warning.title}
													</div>
													<div
														css={{
															fontSize: 13,
															color: theme.palette.text.secondary,
															lineHeight: 1.5,
														}}
													>
														{warning.message}
													</div>
												</div>
											)}
										</TableCell>
										<TableCell>
											{!isDismissed && warning.code && (
												<span
													css={{
														fontSize: 12,
														color: theme.palette.text.secondary,
														fontFamily: "monospace",
														backgroundColor: theme.palette.background.default,
														padding: "2px 8px",
														borderRadius: 4,
														border: `1px solid ${theme.palette.divider}`,
													}}
												>
													{warning.code}
												</span>
											)}
										</TableCell>
										<TableCell>
											<button
												type="button"
												onClick={() => handleToggleDismiss(warning.id)}
												css={{
													background: "none",
													border: "none",
													cursor: "pointer",
													padding: 4,
													display: "flex",
													alignItems: "center",
													justifyContent: "center",
													borderRadius: 4,
													color: theme.palette.text.secondary,
													transition: "all 0.15s ease",
													"&:hover": {
														backgroundColor: theme.palette.action.hover,
														color: theme.palette.text.primary,
													},
												}}
												aria-label={
													isDismissed ? "Restore warning" : "Dismiss warning"
												}
												title={isDismissed ? "Restore" : "Dismiss"}
											>
												{isDismissed ? (
													<RotateCcwIcon size={16} />
												) : (
													<XIcon size={16} />
												)}
											</button>
										</TableCell>
									</TableRow>
								);
							})}
						</TableBody>
					</Table>
				)}
			</div>
		</>
	);
};

export default TemplateWarningsPage;
