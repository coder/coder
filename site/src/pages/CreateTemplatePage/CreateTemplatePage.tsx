import { type FC, useRef, useState } from "react";
import { useMutation } from "react-query";
import { useNavigate, useSearchParams } from "react-router";
import { createTemplate } from "#/api/queries/templates";
import type { TemplateVersion } from "#/api/typesGenerated";
import { FullPageHorizontalForm } from "#/components/FullPageForm/FullPageHorizontalForm";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import { pageTitle } from "#/utils/page";
import { BuildLogsDrawer } from "./BuildLogsDrawer";
import { DuplicateTemplateView } from "./DuplicateTemplateView";
import { ImportStarterTemplateView } from "./ImportStarterTemplateView";
import type { CreateTemplatePageViewProps } from "./types";
import { UploadTemplateView } from "./UploadTemplateView";

const CreateTemplatePage: FC = () => {
	const navigate = useNavigate();
	const getLink = useLinks();
	const [searchParams] = useSearchParams();
	const [isBuildLogsOpen, setIsBuildLogsOpen] = useState(false);
	const [templateVersion, setTemplateVersion] = useState<TemplateVersion>();
	const createTemplateMutation = useMutation(createTemplate());
	const variablesSectionRef = useRef<HTMLDivElement>(null);
	// Remember what had focus when the drawer opened so it can be restored on
	// close. The drawer is opened from buttons outside its tree, so Radix has no
	// trigger to fall back to.
	const buildLogsOpenerRef = useRef<HTMLElement | null>(null);
	// Keeps focus on the variables input (rather than the opener) when the drawer
	// closes via the "Fill variables" action.
	const preserveVariablesFocusRef = useRef(false);

	const openBuildLogs = () => {
		buildLogsOpenerRef.current =
			document.activeElement instanceof HTMLElement
				? document.activeElement
				: null;
		setIsBuildLogsOpen(true);
	};

	const fillVariables = () => {
		variablesSectionRef.current?.scrollIntoView({ behavior: "smooth" });
		preserveVariablesFocusRef.current = true;
		setIsBuildLogsOpen(false);
	};

	const restoreFocusOnDrawerClose = (event: Event) => {
		// Radix focuses a DrawerTrigger on close by default, but this drawer has
		// none. Return focus to the variables input for the "Fill variables" flow,
		// otherwise to whatever opened the drawer, so keyboard users are not
		// dropped onto the document body.
		if (preserveVariablesFocusRef.current) {
			preserveVariablesFocusRef.current = false;
			event.preventDefault();
			variablesSectionRef.current?.querySelector("input")?.focus();
			return;
		}
		const opener = buildLogsOpenerRef.current;
		if (opener?.isConnected) {
			event.preventDefault();
			opener.focus();
		}
	};

	const pageViewProps: CreateTemplatePageViewProps = {
		onCreateTemplate: async (options) => {
			openBuildLogs();
			const template = await createTemplateMutation.mutateAsync({
				...options,
				onCreateVersion: setTemplateVersion,
				onTemplateVersionChanges: setTemplateVersion,
			});
			// Ephemeral router state tells TemplateFilesPage to show a
			// one-time banner with agent skill and docs links.
			navigate(
				`${getLink(linkToTemplate(options.organization, template.name))}/files`,
				{ state: { justCreated: true } },
			);
		},
		onOpenBuildLogsDrawer: openBuildLogs,
		error: createTemplateMutation.error,
		isCreating: createTemplateMutation.isPending,
		variablesSectionRef,
	};

	return (
		<>
			<title>{pageTitle("Create Template")}</title>

			<FullPageHorizontalForm title="Create Template">
				{searchParams.has("fromTemplate") ? (
					<DuplicateTemplateView {...pageViewProps} />
				) : searchParams.has("exampleId") ? (
					<ImportStarterTemplateView {...pageViewProps} />
				) : (
					<UploadTemplateView {...pageViewProps} />
				)}
			</FullPageHorizontalForm>

			<BuildLogsDrawer
				error={createTemplateMutation.error}
				open={isBuildLogsOpen}
				onClose={() => setIsBuildLogsOpen(false)}
				onFillVariables={fillVariables}
				onCloseAutoFocus={restoreFocusOnDrawerClose}
				templateVersion={templateVersion}
			/>
		</>
	);
};

export default CreateTemplatePage;
