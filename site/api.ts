import { SvgIcon } from "@material-ui/core"
import { Logo } from "./components/Icons"
import { wait } from "./util"

export interface Project {
  id: string
  icon?: string
  name: string
  description: string
}

export namespace Project {
  const testProject1: Project = {
    id: "test-terraform-1",
    icon: "https://www.datocms-assets.com/2885/1620155117-brandhcterraformverticalcolorwhite.svg",
    name: "Terraform Project 1",
    description: "Simple terraform project that deploys a kubernetes provider",
  }

  const testProject2: Project = {
    id: "test-echo-1",
    name: "Echo Project",
    description: "Project built on echo provisioner",
  }

  const allProjects = [testProject1, testProject2]

  export const getAllProjectsInOrg = async (_org: string): Promise<Project[]> => {
    await wait(250)
    return allProjects
  }

  export const getProject = async (_org: string, projectId: string): Promise<Project> => {
    await wait(250)

    const matchingProjects = allProjects.filter((p) => p.id === projectId)

    if (matchingProjects.length === 0) {
      throw new Error(`No project matching ${projectId} found`)
    }

    return matchingProjects[0]
  }

  export const createWorkspace = async (name: string): Promise<string> => {
    await wait(250)
    return "test-workspace"
  }
}

export namespace Workspace {
  export type WorkspaceId = string

  export const createWorkspace = (name: string, projectTemplate: string): Promise<WorkspaceId> => {
    return Promise.resolve("test-workspace")
  }
}
