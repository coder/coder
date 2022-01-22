<<<<<<< HEAD
import { wait } from "./util"

// TEMPORARY
// This is all placeholder / stub code until we have a real API to work with!
//
// The implementations below that are hard-coded will switch to using `fetch`
// once the routes are available.
// TEMPORARY

export type ProjectParameterType = "string" | "number"

export interface ProjectParameter {
  id: string
  name: string
  description: string
  defaultValue?: string
  type: ProjectParameterType
}

export interface Project {
  id: string
  icon?: string
  name: string
  description: string
  parameters: ProjectParameter[]
}

export namespace Project {
  const testProject1: Project = {
    id: "test-terraform-1",
    icon: "https://www.datocms-assets.com/2885/1620155117-brandhcterraformverticalcolorwhite.svg",
    name: "Terraform Project 1",
    description: "Simple terraform project that deploys a kubernetes provider",
    parameters: [
      {
        id: "namespace",
        name: "Namespace",
        description: "The kubernetes namespace that will own the workspace pod.",
        defaultValue: "default",
        type: "string",
      },
      {
        id: "cpu_cores",
        name: "CPU Cores",
        description: "Number of CPU cores to allocate for pod.",
        type: "number",
      },
    ],
  }

  const testProject2: Project = {
    id: "test-echo-1",
    name: "Echo Project",
    description: "Project built on echo provisioner",
    parameters: [
      {
        id: "echo_string",
        name: "Echo string",
        description: "String that will be echoed out during build.",
        type: "string",
      },
    ],
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

  export const createWorkspace = (
    name: string,
    projectTemplate: string,
    parameters: Record<string, string>,
  ): Promise<WorkspaceId> => {
    alert(
      `Creating workspace named ${name} for project ${projectTemplate} with parameters: ${JSON.stringify(parameters)}`,
    )

    return Promise.resolve("test-workspace")
  }
}
||||||| 36b7b20
=======
interface LoginResponse {
  session_token: string
}

export const login = async (email: string, password: string): Promise<LoginResponse> => {
  const response = await fetch("/api/v2/login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      email,
      password,
    }),
  })

  const body = await response.json()
  if (!response.ok) {
    throw new Error(body.message)
  }

  return body
}
>>>>>>> main
