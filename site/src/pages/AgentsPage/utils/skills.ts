import frontMatter from "front-matter";

export const SKILL_MAX_SIZE_BYTES = 64 * 1024;
const SKILL_MAX_NAME_BYTES = 256;
const SKILL_MAX_DESCRIPTION_BYTES = 4096;
export const SKILLS_MAX_PER_OWNER = 100;

const skillNamePattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
const textEncoder = new TextEncoder();

export type SkillFormValues = {
	name: string;
	description: string;
	body: string;
};

type SkillSearchMetadata = {
	name: string;
	description: string;
	triggerText?: string;
	// Alternate trigger form that stays searchable regardless of which
	// form is displayed, e.g. the qualified /source/name alias.
	altTriggerText?: string;
};

type RankedSkill<T extends SkillSearchMetadata> = {
	skill: T;
	rank: number;
	index: number;
};

type SkillTriggerMatch = {
	slashOffset: number;
	query: string;
};

export const parseSkillTrigger = (
	linePrefix: string,
): SkillTriggerMatch | null => {
	const match = /(?:^|\s)\/(\S*)$/.exec(linePrefix);
	if (!match) {
		return null;
	}

	return {
		slashOffset: match.index + match[0].indexOf("/"),
		query: match[1] ?? "",
	};
};

export const isSkillTriggerToken = (token: string): boolean =>
	/^\/\S*$/.test(token);

/**
 * Filters skills by name, trigger text, and description. Matches are ranked
 * by name or trigger text prefix, name or trigger text substring, then
 * description substring.
 */
export const filterSkillsByQuery = <T extends SkillSearchMetadata>(
	skills: readonly T[],
	query: string,
): T[] => {
	const normalizedQuery = query.toLocaleLowerCase("en-US");
	if (!normalizedQuery) {
		return skills.toSorted((a, b) => a.name.localeCompare(b.name, "en-US"));
	}

	const rankedSkills: RankedSkill<T>[] = [];
	for (const [index, skill] of skills.entries()) {
		const name = skill.name.toLocaleLowerCase("en-US");
		const triggerText = skill.triggerText
			?.replace(/^\//, "")
			.toLocaleLowerCase("en-US");
		const altTriggerText = skill.altTriggerText
			?.replace(/^\//, "")
			.toLocaleLowerCase("en-US");
		const description = skill.description.toLocaleLowerCase("en-US");
		let rank: number | undefined;
		if (
			name.startsWith(normalizedQuery) ||
			triggerText?.startsWith(normalizedQuery) ||
			altTriggerText?.startsWith(normalizedQuery)
		) {
			rank = 0;
		} else if (
			name.includes(normalizedQuery) ||
			triggerText?.includes(normalizedQuery) ||
			altTriggerText?.includes(normalizedQuery)
		) {
			rank = 1;
		} else if (description.includes(normalizedQuery)) {
			rank = 2;
		}
		if (rank !== undefined) {
			rankedSkills.push({ skill, rank, index });
		}
	}

	return rankedSkills
		.toSorted((a, b) => {
			if (a.rank !== b.rank) {
				return a.rank - b.rank;
			}
			const nameOrder = a.skill.name.localeCompare(b.skill.name, "en-US");
			return nameOrder === 0 ? a.index - b.index : nameOrder;
		})
		.map(({ skill }) => skill);
};

class SkillMarkdownError extends Error {}

const frontmatterStringField = (
	attributes: Record<string, unknown>,
	key: "name" | "description",
): string => {
	const value = attributes[key];
	if (value === undefined) {
		return "";
	}
	if (typeof value !== "string") {
		throw new SkillMarkdownError(`Skill ${key} must be a string.`);
	}
	return value.replace(/[\r\n]+$/, "");
};

// The API re-validates on submit; this only projects content into form fields.
export const parseSkillMarkdown = (content: string): SkillFormValues => {
	const normalizedContent = content.replace(/^\uFEFF/, "");
	const lines = normalizedContent.split("\n");
	if (lines[0]?.trim() !== "---") {
		throw new SkillMarkdownError("Missing opening frontmatter delimiter.");
	}

	const closingIndex = lines.findIndex(
		(line, index) => index > 0 && line.trim() === "---",
	);
	if (closingIndex < 0) {
		throw new SkillMarkdownError("Missing closing frontmatter delimiter.");
	}

	const parseableContent = [
		"---",
		...lines.slice(1, closingIndex),
		"---",
		...lines.slice(closingIndex + 1),
	].join("\n");
	const parsed = (() => {
		try {
			return frontMatter<Record<string, unknown>>(parseableContent);
		} catch (error) {
			const message = error instanceof Error ? error.message : "unknown error";
			throw new SkillMarkdownError(`Invalid frontmatter: ${message}`);
		}
	})();

	const name = frontmatterStringField(parsed.attributes, "name");
	const description = frontmatterStringField(parsed.attributes, "description");
	const body = parsed.body.trim();

	if (!name) {
		throw new SkillMarkdownError("Skill name is required.");
	}
	if (!body) {
		throw new SkillMarkdownError("Skill body is required.");
	}

	return { name, description, body };
};

export const tryParseSkillMarkdown = (
	content: string,
): { ok: true; values: SkillFormValues } | { ok: false; error: string } => {
	try {
		return { ok: true, values: parseSkillMarkdown(content) };
	} catch (error) {
		return {
			ok: false,
			error:
				error instanceof Error ? error.message : "Unable to parse SKILL.md.",
		};
	}
};

const frontmatterLineValue = (value: string): string =>
	value.replace(/\r?\n/g, " ").trim();

const frontmatterStringValue = (value: string): string =>
	`"${frontmatterLineValue(value).replace(/\\/g, "\\\\").replace(/"/g, '\\"')}"`;

const frontmatterNameValue = (value: string): string => {
	const lineValue = frontmatterLineValue(value);
	if (/^(?:true|false|null)$/.test(lineValue) || /^[0-9]/.test(lineValue)) {
		return frontmatterStringValue(lineValue);
	}
	return lineValue;
};

export const isValidSkillDescription = (description: string): boolean =>
	getSkillContentSizeBytes(description) <= SKILL_MAX_DESCRIPTION_BYTES;

export const buildSkillMarkdown = (values: SkillFormValues): string => {
	const name = frontmatterNameValue(values.name);
	const description = frontmatterLineValue(values.description);
	const body = values.body.trim();
	const frontmatter = ["---", `name: ${name}`];
	if (description) {
		frontmatter.push(`description: ${frontmatterStringValue(description)}`);
	}
	frontmatter.push("---");

	return `${frontmatter.join("\n")}\n${body}\n`;
};

export const getSkillContentSizeBytes = (content: string): number =>
	textEncoder.encode(content).length;

export const isValidSkillName = (name: string): boolean =>
	skillNamePattern.test(name) &&
	getSkillContentSizeBytes(name) <= SKILL_MAX_NAME_BYTES;

export type SkillsCopy = {
	/** Singular noun in sentence case, for example "Personal skill". */
	noun: string;
	title: string;
	description: string;
	emptyDescription: string;
	editorDescription: string;
	archiveName: string;
};

export type SkillAccess = {
	create: boolean;
	update: boolean;
	delete: boolean;
};

export const fullSkillAccess: SkillAccess = {
	create: true,
	update: true,
	delete: true,
};

export const readOnlySkillAccess: SkillAccess = {
	create: false,
	update: false,
	delete: false,
};
