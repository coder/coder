import type { Annotation, AnnotationSubmission } from "./protocol";

function quote(value: string): string {
	return `"${value.replaceAll('"', '\\"')}"`;
}

function formatAnnotation(annotation: Annotation, index: number): string {
	const { element } = annotation;
	const lines = [
		`## ${index + 1}. ${annotation.comment.trim() || "(no comment)"}`,
		"",
	];
	lines.push(`- Element: \`${element.selector}\` (\`<${element.tag}>\`)`);
	if (element.testId) {
		lines.push(`- Test id: \`${element.testId}\``);
	}
	if (element.role || element.ariaLabel) {
		const parts = [
			element.role ? `role=${quote(element.role)}` : undefined,
			element.ariaLabel ? `aria-label=${quote(element.ariaLabel)}` : undefined,
		].filter((part) => part !== undefined);
		lines.push(`- Accessibility: ${parts.join(", ")}`);
	}
	if (element.text) {
		lines.push(`- Text: ${quote(element.text)}`);
	}
	if (annotation.selectedText) {
		lines.push(`- Selected text: ${quote(annotation.selectedText)}`);
	}
	if (element.reactComponents?.length) {
		lines.push(`- React: ${element.reactComponents.join(" < ")}`);
	}
	if (element.sourceLocation) {
		lines.push(`- Source: \`${element.sourceLocation}\``);
	}
	if (element.classes.length > 0) {
		lines.push(`- Classes: \`${element.classes.join(" ")}\``);
	}
	lines.push(
		`- Position: ${element.rect.width}x${element.rect.height} at (${element.rect.x}, ${element.rect.y})`,
	);
	lines.push(`- Tag: \`${element.openingTag}\``);
	return lines.join("\n");
}

/**
 * Renders a submission as markdown that reads well in the chat transcript
 * and gives an agent greppable handles (selectors, test ids, component
 * names, source locations) for every annotated element.
 */
export function formatAnnotations(submission: AnnotationSubmission): string {
	const { page, annotations } = submission;
	const header = [
		"# UI annotations",
		"",
		`Page: ${page.url}${page.title ? ` (${page.title})` : ""}`,
		`Viewport: ${page.viewport.width}x${page.viewport.height}`,
		`Count: ${annotations.length}`,
		"",
		"",
	];
	const body = annotations.map(formatAnnotation).join("\n\n");
	return `${header.join("\n")}${body}\n`;
}
