export interface Project {
  id: string
  name: string
  description: string
}

export namespace Project {
  export const get = async (org: string): Promise<Project[]> => {
    const project1: Project = {
      id: "test-terraform-1",
      name: "Terraform Project 1",
      description: "Simple terraform project that deploys a kubernetes provider",
    }

    const project2: Project = {
      id: "test-echo-1",
      name: "Echo Project",
      description: "Project built on echo provisioner",
    }

    return Promise.resolve([project1, project2])
  }
}
