import { useQuery } from "react-query";

const ICON_PREFIX = "/icon/";
const EMOJI_PREFIX = "/emojis/";

/**
 * Product names that cannot be recovered by title-casing the icon file name.
 */
const ICON_LABEL_OVERRIDES: Record<string, string> = {
	"amazon-q": "Amazon Q",
	"android-studio": "Android Studio",
	"apache-guacamole": "Apache Guacamole",
	"argo-workflows": "Argo Workflows",
	"azure-devops": "Azure DevOps",
	aws: "AWS",
	centos: "CentOS",
	clion: "CLion",
	code: "Visual Studio Code",
	"code-insiders": "Visual Studio Code Insiders",
	cpp: "C++",
	datagrip: "DataGrip",
	dataspell: "DataSpell",
	dcv: "Amazon DCV",
	devcontainers: "Dev Containers",
	do: "DigitalOcean",
	dotnet: ".NET",
	filebrowser: "File Browser",
	"fly.io": "Fly.io",
	gcp: "Google Cloud",
	github: "GitHub",
	"github-copilot": "GitHub Copilot",
	gitlab: "GitLab",
	goland: "GoLand",
	intellij: "IntelliJ IDEA",
	javascript: "JavaScript",
	jax: "JAX",
	jetbrains: "JetBrains",
	"jetbrains-toolbox": "JetBrains Toolbox",
	jfrog: "JFrog",
	k8s: "Kubernetes",
	kasmvnc: "KasmVNC",
	lakefs: "lakeFS",
	lxc: "LXC",
	matlab: "MATLAB",
	"microsoft-teams": "Microsoft Teams",
	mlflow: "MLflow",
	node: "Node.js",
	nodejs: "Node.js",
	novnc: "noVNC",
	openai: "OpenAI",
	"openai-codex": "OpenAI Codex",
	opencode: "OpenCode",
	openrouter: "OpenRouter",
	openwebui: "Open WebUI",
	php: "PHP",
	phpstorm: "PhpStorm",
	postgres: "PostgreSQL",
	pycharm: "PyCharm",
	pytorch: "PyTorch",
	rdp: "Remote Desktop",
	rockylinux: "Rocky Linux",
	rproject: "R",
	rstudio: "RStudio",
	rubymine: "RubyMine",
	rustdesk: "RustDesk",
	rustrover: "RustRover",
	tensorflow: "TensorFlow",
	texlive: "TeX Live",
	tmux: "tmux",
	typescript: "TypeScript",
	vsphere: "vSphere",
	webstorm: "WebStorm",
	almalinux: "AlmaLinux",
};

/** Color and style variants that share a label with the base icon. */
const ICON_VARIANT_SUFFIX = /-(dark|light|monochrome|white|black|grey)$/;

/** Built-in icons that depict a generic concept rather than a brand. */
const GENERIC_ICONS = new Set([
	"auto-dev-server",
	"container",
	"database",
	"desktop",
	"dotfiles",
	"folder",
	"image",
	"memory",
	"personalize",
	"rdp",
	"tasks",
	"terminal",
	"widgets",
]);

const isBuiltinEmojiSrc = (src: string | undefined): src is string =>
	Boolean(src?.startsWith(EMOJI_PREFIX));

/** Maps an emoji's unified code point sequence, such as `1f34c`, to its name. */
type EmojiNames = Readonly<Record<string, string>>;

const getIconAlt = (src: string): string => {
	const fileName = src.slice(ICON_PREFIX.length).split(/[?#]/)[0];
	const id = fileName
		.replace(/\.(svg|png)$/, "")
		.replace(ICON_VARIANT_SUFFIX, "");
	if (!id) {
		return "";
	}
	const label =
		ICON_LABEL_OVERRIDES[id] ??
		id
			.split(/[-_]/)
			.filter(Boolean)
			.map((word) => word.charAt(0).toUpperCase() + word.slice(1))
			.join(" ");
	return GENERIC_ICONS.has(id) ? `${label} icon` : `${label} logo`;
};

const getEmojiAlt = (src: string, emojiNames: EmojiNames | undefined) => {
	const unified = src.slice(EMOJI_PREFIX.length).replace(/\.png$/, "");
	const name =
		emojiNames?.[unified] ?? emojiNames?.[unified.replace(/-fe0f/g, "")];
	return name ? `${name} emoji` : "Emoji";
};

/** Alt text for built-in icons and emojis, or `""` for any other source. */
export const getBuiltinImageAlt = (
	src: string | undefined,
	emojiNames?: EmojiNames,
): string => {
	if (src?.startsWith(ICON_PREFIX)) {
		return getIconAlt(src);
	}
	if (isBuiltinEmojiSrc(src)) {
		return getEmojiAlt(src, emojiNames);
	}
	return "";
};

const builtinEmojiNamesKey = ["builtinEmojiNames"] as const;

const loadEmojiNames = async (): Promise<EmojiNames> => {
	const { default: data } = await import("@emoji-mart/data/sets/15/apple.json");
	const names: Record<string, string> = {};
	for (const emoji of Object.values(data.emojis)) {
		for (const skin of emoji.skins) {
			names[skin.unified] = emoji.name;
		}
	}
	return names;
};

/** Loads emoji names only when `srcs` contains a built-in emoji. */
export const useBuiltinImageAlt = (
	srcs: readonly (string | undefined)[],
	enabled = true,
) => {
	const needsEmojiNames = enabled && srcs.some(isBuiltinEmojiSrc);
	const emojiNamesQuery = useQuery({
		queryKey: builtinEmojiNamesKey,
		queryFn: loadEmojiNames,
		enabled: needsEmojiNames,
		staleTime: Number.POSITIVE_INFINITY,
	});

	return {
		isLoading: needsEmojiNames && !emojiNamesQuery.isFetched,
		getAlt: (src: string | undefined) =>
			getBuiltinImageAlt(src, emojiNamesQuery.data),
	};
};
