import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { chromatic } from "#/testHelpers/chromatic";
import type { FileTree } from "#/utils/filetree";
import { TemplateFileTree } from "./TemplateFileTree";

const fileTree: FileTree = {
	"boundary-config.yaml": "- secure: yup",
	"configure-chrome-flags.sh": "#!/bin/bash",
	Dockerfile: "FROM ubuntu:26.04",
	files: {
		etc: {
			apt: {
				"sources.list.d": {
					"ppa.list": "wow you found my secret hiding spot",
				},
			},
		},
		usr: {
			local: {
				bin: {
					gh: "#!/bin/bash",
				},
			},
		},
	},
	".env": "TOKEN=1",
	"main.tf": 'resource "wibble" "wobble" {}',
	Makefile: "build:\n\tgo build\n.PHONY: build",
	"README.md": "# Congratulations on being able to read",
	"update-keys.sh": "#!/bin/bash",
};

const meta: Meta<typeof TemplateFileTree> = {
	title: "modules/templates/TemplateFileTree",
	parameters: { chromatic },
	component: TemplateFileTree,
	args: {
		fileTree,
		activePath: "main.tf",
		onDelete: action("delete"),
		onRename: action("rename"),
	},
	decorators: [
		(Story) => {
			return (
				<div className="max-w-[260px] rounded-lg border border-solid border-border-default">
					<Story />
				</div>
			);
		},
	],
};

export default meta;
type Story = StoryObj<typeof TemplateFileTree>;

export const Example: Story = {};

export const NestedOpen: Story = {
	args: {
		activePath: "folder/nested.tf",
	},
};

export const GroupEmptyFolders: Story = {
	args: {
		activePath: "folder/other-folder/another/nested.tf",
		fileTree: {
			"main.tf": "resource aws_instance my_instance {}",
			"variables.tf": "variable my_var {}",
			"outputs.tf": "output my_output {}",
			folder: {
				"other-folder": {
					another: {
						"nested.tf": "resource aws_instance my_instance {}",
					},
				},
			},
		},
	},
};

export const GreyOutHiddenFiles: Story = {
	args: {
		fileTree: {
			".vite": {
				"config.json": "resource aws_instance my_instance {}",
			},
			".nextjs": {
				"nested.tf": "resource aws_instance my_instance {}",
			},
			".terraform.lock.hcl": "{}",
			"main.tf": "resource aws_instance my_instance {}",
			"variables.tf": "variable my_var {}",
			"outputs.tf": "output my_output {}",
		},
	},
};
